package migrate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/degoke/outpost/internal/authz"
	"github.com/degoke/outpost/internal/bootstrap"
	"github.com/degoke/outpost/internal/cleanup"
	"github.com/degoke/outpost/internal/cluster"
	"github.com/degoke/outpost/internal/compose"
	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/connect"
	"github.com/degoke/outpost/internal/environment"
	"github.com/degoke/outpost/internal/machine"
	"github.com/degoke/outpost/internal/mirror"
	"github.com/degoke/outpost/internal/output"
	"github.com/degoke/outpost/internal/project"
	"github.com/degoke/outpost/internal/transport"
)

// ExecutorFactory creates a transport executor for a named host.
type ExecutorFactory func(hostName string) (transport.Executor, *config.Host, error)

// Options configures a project migration between hosts.
type Options struct {
	Cwd         string
	Project     *config.Project
	Global      *config.Global
	FromHost    string
	ToHost      string
	ForceYes    bool
	DryRun      bool
	SkipVolumes bool
	SkipCluster bool
	SkipMachine bool
	SkipCompose bool
	HostFlag    string
	Out         *output.Printer
	NewExecutor ExecutorFactory
}

// Plan describes the steps migrate will run.
type Plan struct {
	Project  string   `json:"project"`
	FromHost string   `json:"from_host"`
	ToHost   string   `json:"to_host"`
	Steps    []string `json:"steps"`
	Warnings []string `json:"warnings,omitempty"`
}

// Result summarizes a completed migration.
type Result struct {
	Project          string   `json:"project"`
	FromHost         string   `json:"from_host"`
	ToHost           string   `json:"to_host"`
	DockerExported   bool     `json:"docker_exported"`
	DockerImported   bool     `json:"docker_imported"`
	MachineExported  bool     `json:"machine_exported"`
	MachineImported  bool     `json:"machine_imported"`
	RemoteStateMoved bool     `json:"remote_state_moved"`
	ComposeStarted   bool     `json:"compose_started"`
	Warnings         []string `json:"warnings,omitempty"`
}

// Run migrates the project's remote environment from one host to another.
func Run(ctx context.Context, opts Options) (*Result, error) {
	if opts.Project == nil {
		return nil, fmt.Errorf("project is required")
	}
	if opts.NewExecutor == nil {
		return nil, fmt.Errorf("executor factory is required")
	}
	if strings.TrimSpace(opts.ToHost) == "" {
		return nil, fmt.Errorf("--to is required")
	}

	fromHost := strings.TrimSpace(opts.FromHost)
	if fromHost == "" {
		fromHost = project.ResolveHostName(opts.HostFlag, opts.Project.Host)
	}
	if fromHost == "" && opts.Global != nil {
		fromHost = opts.Global.ActiveHost
	}
	if fromHost == "" {
		return nil, fmt.Errorf("source host is required — pass --from or set host in project.yaml")
	}
	toHost := strings.TrimSpace(opts.ToHost)
	if fromHost == toHost {
		return nil, fmt.Errorf("source and destination hosts must differ")
	}

	if opts.Global != nil {
		if _, err := opts.Global.ResolveHost(fromHost); err != nil {
			return nil, fmt.Errorf("source host: %w", err)
		}
		if _, err := opts.Global.ResolveHost(toHost); err != nil {
			return nil, fmt.Errorf("destination host: %w", err)
		}
	}

	_, fromH, err := opts.NewExecutor(fromHost)
	if err != nil {
		return nil, err
	}
	if err := authz.RequireOwner(fromH, "migrate"); err != nil {
		return nil, err
	}
	_, toH, err := opts.NewExecutor(toHost)
	if err != nil {
		return nil, err
	}
	if err := authz.RequireOwner(toH, "migrate"); err != nil {
		return nil, err
	}

	plan := buildPlan(opts, fromHost, toHost)
	if opts.DryRun {
		if opts.Out != nil && opts.Out.JSON {
			return nil, opts.Out.PrintJSON(plan)
		}
		if opts.Out != nil {
			opts.Out.Info("Migration plan for project %q: %s -> %s", opts.Project.Name, fromHost, toHost)
			for _, step := range plan.Steps {
				opts.Out.Info("  - %s", step)
			}
			for _, warning := range plan.Warnings {
				opts.Out.Info("  ! %s", warning)
			}
		}
		return &Result{Project: opts.Project.Name, FromHost: fromHost, ToHost: toHost, Warnings: plan.Warnings}, nil
	}

	msg := fmt.Sprintf("migrate project %q from host %q to %q", opts.Project.Name, fromHost, toHost)
	if err := authz.ConfirmWithYes(msg, opts.ForceYes); err != nil {
		return nil, err
	}
	if err := checkMigratePreflight(fromHost, opts.Project.Name); err != nil {
		return nil, err
	}

	result := &Result{
		Project:  opts.Project.Name,
		FromHost: fromHost,
		ToHost:   toHost,
		Warnings: plan.Warnings,
	}

	var bundleManifest *DockerBundleManifest
	bundleOpts := dockerBundleOptions{SkipCluster: opts.SkipCluster}

	_ = connect.StopSession(toHost, opts.Project.Name)
	_ = os.Remove(cluster.LocalProjectKubeconfigPath(opts.Cwd))

	if err := onHost(ctx, opts, fromHost, func(ctx context.Context, exec transport.Executor, h *config.Host) error {
		if err := checkRemoteSessions(ctx, exec, opts.Project); err != nil {
			return err
		}
		if err := stopComposeStack(ctx, opts, exec, fromHost); err != nil {
			return err
		}
		if !opts.SkipVolumes {
			if err := ensureSourceDevContainer(ctx, exec, opts.Cwd, opts.Project, opts.Out); err != nil {
				return err
			}
			exported, err := exportDockerBundle(ctx, exec, opts.Cwd, opts.Project, opts.Out, bundleOpts)
			if err != nil {
				return err
			}
			result.DockerExported = exported
			if w := appContainerWarning(opts.Cwd, opts.Project, ctx, exec); w != "" {
				result.Warnings = append(result.Warnings, w)
			}
		}
		if !opts.SkipMachine && opts.Project.Machine != nil {
			exported, err := exportMachine(ctx, exec, opts.Project, opts.Out)
			if err != nil {
				return err
			}
			result.MachineExported = exported
		}
		moved, err := exportRemoteState(ctx, exec, opts.Project)
		if err != nil {
			return err
		}
		result.RemoteStateMoved = moved
		cleanupStaging(ctx, exec, opts.Project)
		return nil
	}); err != nil {
		return nil, err
	}

	originalHost := opts.Project.Host

	if err := onHost(ctx, opts, toHost, func(ctx context.Context, exec transport.Executor, h *config.Host) error {
		cleanupOpts := cleanup.OptionsForProject(opts.Project)
		cleanupOpts.IncludeDockerCache = false
		if err := cleanup.Project(ctx, exec, opts.Project, cleanupOpts); err != nil {
			return err
		}

		if opts.Out != nil {
			opts.Out.Step("Syncing repository to %s...", toHost)
		}
		if _, err := mirror.New(exec, opts.Project, opts.Cwd, toHost, opts.Out).SyncIfNeeded(ctx, true); err != nil {
			return err
		}

		if moved, err := importRemoteState(ctx, exec, opts.Project); err != nil {
			return err
		} else if moved {
			result.RemoteStateMoved = true
		}

		if !opts.SkipVolumes && result.DockerExported {
			manifest, err := importDockerBundle(ctx, exec, opts.Project, opts.Out)
			if err != nil {
				return err
			}
			if manifest != nil {
				result.DockerImported = true
				bundleManifest = manifest
			}
		}

		if !opts.SkipMachine && opts.Project.Machine != nil {
			imported, err := importMachine(ctx, exec, opts.Project, h.Provider, opts.Out)
			if err != nil {
				return err
			}
			if imported {
				result.MachineImported = true
			} else if !localArchiveExists(opts.Project.Name, machineArchiveName) {
				if opts.Out != nil {
					opts.Out.Step("Creating project Incus machine on %s...", toHost)
				}
				svc := machine.NewProjectService(exec, opts.Project, opts.Out)
				if err := svc.Up(ctx, machineOptionsFromProject(opts.Project), h.Provider); err != nil {
					return err
				}
				result.MachineImported = true
			}
		}

		kubernetesRestored := bundleManifest != nil && bundleManifest.KubernetesState != ""
		containersRestored := bundleManifest != nil && bundleManifest.ContainerCount > 0

		if !opts.SkipCluster && opts.Project.Kubernetes != nil && !kubernetesRestored {
			if opts.Out != nil {
				opts.Out.Step("Creating project Kubernetes cluster on %s...", toHost)
			}
			driver := ""
			if opts.Project.Kubernetes.Driver != "" {
				driver = opts.Project.Kubernetes.Driver
			}
			if err := cluster.NewProjectService(exec, opts.Project, opts.Cwd, opts.Out).Up(ctx, driver); err != nil {
				return err
			}
		}

		if opts.Project.EnvironmentEnabled() {
			if opts.Out != nil {
				opts.Out.Step("Ensuring project development container on %s...", toHost)
			}
			if err := environment.New(exec, opts.Project, opts.Cwd).Ensure(ctx); err != nil {
				return err
			}
		}

		if !opts.SkipCompose && opts.Project.RequireCompose() == nil && !containersRestored {
			if opts.Out != nil {
				opts.Out.Step("Starting compose stack on %s...", toHost)
			}
			runner := &compose.Runner{
				Exec:     exec,
				Project:  opts.Project,
				Cwd:      opts.Cwd,
				HostName: toHost,
				ForceYes: opts.ForceYes,
				Out:      opts.Out,
			}
			code, err := runner.Run(ctx, "up", []string{"-d"}, false)
			if err != nil {
				return err
			}
			if code != 0 {
				return fmt.Errorf("compose up failed (exit %d)", code)
			}
			result.ComposeStarted = true
		}
		cleanupStaging(ctx, exec, opts.Project)
		return nil
	}); err != nil {
		return nil, partialFailureHint(result, opts.Project.Name, err)
	}

	opts.Project.Host = toHost
	if err := config.SaveProject(opts.Cwd, opts.Project); err != nil {
		return nil, err
	}
	if opts.Out != nil && originalHost != toHost {
		opts.Out.Step("Updated project host to %q", toHost)
	}

	if opts.Out != nil {
		if opts.Out.JSON {
			return result, opts.Out.PrintJSON(result)
		}
		opts.Out.Success("Migrated project %q to host %q", opts.Project.Name, toHost)
		for _, warning := range result.Warnings {
			opts.Out.Info("Note: %s", warning)
		}
		opts.Out.Info("Run 'outpost open' to forward ports from the new host")
	}
	return result, nil
}

func buildPlan(opts Options, fromHost, toHost string) Plan {
	steps := []string{
		fmt.Sprintf("verify no active port forwarding for project %q", opts.Project.Name),
		fmt.Sprintf("verify no remote mirror sessions on %q", fromHost),
		fmt.Sprintf("export project Docker bundle from %q (containers, volumes, Kubernetes)", fromHost),
		fmt.Sprintf("export project Incus machine from %q (if present)", fromHost),
		fmt.Sprintf("export remote .outpost metadata from %q", fromHost),
		fmt.Sprintf("sync repository to %q", toHost),
		fmt.Sprintf("import Docker bundle, Incus machine, and remote metadata on %q", toHost),
		fmt.Sprintf("update .outpost/project.yaml host to %q", toHost),
	}
	if opts.Project.EnvironmentEnabled() {
		steps = append(steps, fmt.Sprintf("ensure development container on %q (if not restored from bundle)", toHost))
	}
	if !opts.SkipCompose && opts.Project.RequireCompose() == nil {
		steps = append(steps, fmt.Sprintf("run compose up on %q", toHost))
	}

	var warnings []string
	warnings = append(warnings, "Close port forwarding with 'outpost close' before migrating")
	if fileExists(filepath.Join(opts.Cwd, "Dockerfile")) {
		warnings = append(warnings, "Run 'outpost app run --detach' on the source host if you need the application container migrated")
	}
	if opts.Project.Kubernetes != nil && !opts.SkipCluster {
		warnings = append(warnings, "Kubernetes is migrated inside the Docker bundle; verify workloads after migration")
	}

	if opts.SkipVolumes {
		steps = filterSteps(steps, "Docker bundle")
	}
	if opts.SkipCluster {
		steps = filterSteps(steps, "Kubernetes")
		warnings = filterWarnings(warnings, "Kubernetes")
	}
	if opts.SkipMachine {
		steps = filterSteps(steps, "Incus machine")
	}
	if opts.SkipCompose {
		steps = filterSteps(steps, "compose up")
	}

	return Plan{
		Project:  opts.Project.Name,
		FromHost: fromHost,
		ToHost:   toHost,
		Steps:    steps,
		Warnings: warnings,
	}
}

func filterSteps(steps []string, substrs ...string) []string {
	var out []string
	for _, step := range steps {
		skip := false
		for _, sub := range substrs {
			if strings.Contains(step, sub) {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, step)
		}
	}
	return out
}

func filterWarnings(warnings []string, substrs ...string) []string {
	var out []string
	for _, warning := range warnings {
		skip := false
		for _, sub := range substrs {
			if strings.Contains(warning, sub) {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, warning)
		}
	}
	return out
}

func localArchiveExists(projectName, archiveName string) bool {
	path, err := localArchiveFile(projectName, archiveName)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

func stopComposeStack(ctx context.Context, opts Options, exec transport.Executor, hostName string) error {
	if opts.Project.RequireCompose() != nil {
		return nil
	}
	runner := &compose.Runner{
		Exec:     exec,
		Project:  opts.Project,
		Cwd:      opts.Cwd,
		HostName: hostName,
		ForceYes: true,
		Out:      opts.Out,
	}
	code, err := runner.Run(ctx, "stop", nil, false)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("compose stop failed (exit %d)", code)
	}
	return nil
}

func onHost(ctx context.Context, opts Options, hostName string, run func(context.Context, transport.Executor, *config.Host) error) error {
	exec, h, err := opts.NewExecutor(hostName)
	if err != nil {
		return err
	}
	if c, ok := exec.(*transport.SSHExecutor); ok {
		defer c.Close()
	}
	if err := bootstrap.EnsureWithOut(ctx, exec, opts.Out); err != nil {
		return err
	}
	if err := authz.RequireRuntimeAccess(ctx, h, exec); err != nil {
		return err
	}
	return run(ctx, exec, h)
}

func machineOptionsFromProject(proj *config.Project) machine.CreateOptions {
	opts := machine.CreateOptions{}
	if proj == nil || proj.Machine == nil {
		return opts
	}
	opts.Image = proj.Machine.Image
	opts.CPU = proj.Machine.CPU
	opts.VirtualMachine = proj.Machine.VirtualMachine
	if proj.Machine.Memory != "" {
		if v, err := machine.ParseSize(proj.Machine.Memory); err == nil {
			opts.MemoryBytes = v
		}
	}
	if proj.Machine.Disk != "" {
		if v, err := machine.ParseSize(proj.Machine.Disk); err == nil {
			opts.DiskBytes = v
		}
	}
	return opts
}
