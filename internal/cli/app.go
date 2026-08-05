package cli

import (
	"context"
	"fmt"
	"os"
	osexec "os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	appsvc "github.com/degoke/outpost/internal/app"
	"github.com/degoke/outpost/internal/authz"
	"github.com/degoke/outpost/internal/bootstrap"
	"github.com/degoke/outpost/internal/capabilities"
	"github.com/degoke/outpost/internal/capacity"
	"github.com/degoke/outpost/internal/cleanup"
	"github.com/degoke/outpost/internal/cluster"
	"github.com/degoke/outpost/internal/compose"
	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/connect"
	"github.com/degoke/outpost/internal/disk"
	"github.com/degoke/outpost/internal/docker"
	"github.com/degoke/outpost/internal/environment"
	"github.com/degoke/outpost/internal/host"
	"github.com/degoke/outpost/internal/machine"
	"github.com/degoke/outpost/internal/migrate"
	"github.com/degoke/outpost/internal/mirror"
	"github.com/degoke/outpost/internal/output"
	"github.com/degoke/outpost/internal/project"
	"github.com/degoke/outpost/internal/prune"
	"github.com/degoke/outpost/internal/share"
	"github.com/degoke/outpost/internal/status"
	"github.com/degoke/outpost/internal/top"
	"github.com/degoke/outpost/internal/transport"
	"github.com/degoke/outpost/internal/upload"
	"github.com/spf13/cobra"
)

type App struct {
	Global          *config.Global
	Out             *output.Printer
	HostFlag        string
	Cwd             string
	ForceYes        bool
	executorFactory ExecutorFactory
	commandPath     string
	commandArgs     []string
}

// ExecutorFactory creates a transport executor for CLI commands. Tests inject a mock factory.
type ExecutorFactory func(g *config.Global, hostName string, autoTrustHostKey bool) (transport.Executor, *config.Host, error)

func New() *cobra.Command {
	root, _ := NewWithApp()
	return root
}

// NewWithApp builds the CLI root command and returns the app for test configuration.
func NewWithApp() (*cobra.Command, *App) {
	app := &App{
		Out: output.New(false, false),
		Cwd: mustCwd(),
	}
	return app.buildRoot(), app
}

func (app *App) buildRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "outpost",
		Short: "Turn a remote Linux host into a shared development runtime",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			g, err := config.LoadGlobal()
			if err != nil {
				return err
			}
			app.Global = g
			jsonOut, _ := cmd.Flags().GetBool("json")
			debug, _ := cmd.Flags().GetBool("debug")
			app.Out = output.New(jsonOut, debug)
			app.HostFlag, _ = cmd.Flags().GetString("host")
			app.ForceYes, _ = cmd.Flags().GetBool("yes")
			app.commandPath = commandPath(cmd)
			app.commandArgs = append([]string(nil), args...)
			hostName := app.HostFlag
			if hostName == "" {
				hostName = g.ActiveHost
			}
			var h *config.Host
			if hostName != "" {
				h, _ = g.ResolveHost(hostName)
			}
			if err := authz.RequireMemberAllowedArgs(h, commandPath(cmd), args); err != nil {
				return err
			}
			return nil
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().Bool("json", false, "machine-readable JSON output")
	root.PersistentFlags().Bool("debug", false, "debug logging")
	root.PersistentFlags().String("host", "", "target host name")
	root.PersistentFlags().Bool("yes", false, "skip confirmation prompts")

	root.AddCommand(app.hostCmd())
	root.AddCommand(app.hostUseAliasCmd())
	root.AddCommand(app.providerCmd())
	root.AddCommand(app.initCmd())
	root.AddCommand(app.dockerCmd())
	root.AddCommand(app.composeCmd())
	root.AddCommand(app.appCmd())
	root.AddCommand(app.clusterCmd())
	root.AddCommand(app.machineCmd())
	root.AddCommand(app.inviteCmd())
	root.AddCommand(app.statusCmd())
	root.AddCommand(app.topCmd())
	root.AddCommand(app.capacityCmd())
	root.AddCommand(app.diskCmd())
	root.AddCommand(app.pruneCmd())
	root.AddCommand(app.resetCmd())
	// Project-first commands.
	root.AddCommand(app.projectShellCmd())
	root.AddCommand(app.projectRunCmd())
	root.AddCommand(app.sessionCmd())
	root.AddCommand(app.projectAICmd())
	root.AddCommand(app.projectOpenCmd())
	root.AddCommand(app.projectCloseCmd())
	root.AddCommand(app.cleanupCmd())
	root.AddCommand(app.migrateCmd())

	return root
}

func (app *App) newExecutor(hostName string) (transport.Executor, *config.Host, error) {
	factory := app.executorFactory
	if factory == nil {
		factory = host.NewExecutor
	}
	return factory(app.Global, hostName, app.ForceYes)
}

// SetExecutorFactory overrides how CLI commands obtain a remote executor (used in tests).
func (app *App) SetExecutorFactory(factory ExecutorFactory) {
	app.executorFactory = factory
}

func commandPath(cmd *cobra.Command) string {
	var parts []string
	for c := cmd; c != nil; c = c.Parent() {
		if c.Use == "" || c.Use == "outpost" {
			continue
		}
		use := strings.Fields(c.Use)[0]
		parts = append([]string{use}, parts...)
	}
	return strings.Join(parts, " ")
}

func mustCwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

func (app *App) resolveHostName(proj *config.Project) string {
	if app.HostFlag != "" {
		return app.HostFlag
	}
	if proj != nil && proj.Host != "" {
		return proj.Host
	}
	return app.Global.ActiveHost
}

func (app *App) withExecutor(run func(context.Context, transport.Executor, *config.Host) error) error {
	exec, h, err := app.newExecutor(app.HostFlag)
	if err != nil {
		return err
	}
	if c, ok := exec.(*transport.SSHExecutor); ok {
		defer c.Close()
	}
	ctx := context.Background()
	if err := authz.RequireRuntimeAccess(ctx, h, exec); err != nil {
		return err
	}
	if err := authz.RequireMemberAllowedArgs(h, app.commandPath, app.commandArgs); err != nil {
		return err
	}
	if h.Role != config.RoleMember {
		if err := bootstrap.EnsureWithOut(ctx, exec, app.Out); err != nil {
			return err
		}
	}
	return run(ctx, exec, h)
}

func (app *App) withProjectExecutor(run func(context.Context, transport.Executor, *config.Host, *config.Project) error) error {
	proj, err := config.LoadProject(app.Cwd)
	if err != nil {
		return err
	}
	hostName := app.resolveHostName(proj)
	exec, h, err := app.newExecutor(hostName)
	if err != nil {
		return err
	}
	if c, ok := exec.(*transport.SSHExecutor); ok {
		defer c.Close()
	}
	ctx := context.Background()
	if err := authz.RequireRuntimeAccess(ctx, h, exec); err != nil {
		return err
	}
	if err := authz.RequireMemberAllowedArgs(h, app.commandPath, app.commandArgs); err != nil {
		return err
	}
	if h.Role != config.RoleMember {
		if err := bootstrap.EnsureWithOut(ctx, exec, app.Out); err != nil {
			return err
		}
	}
	if h.Role != config.RoleMember {
		cleanupOpts := cleanup.OptionsForProject(proj)
		cleanupOpts.IncludeDockerCache = false
		if err := cleanup.Project(ctx, exec, proj, cleanupOpts); err != nil {
			return err
		}
	}
	return run(ctx, exec, h, proj)
}

func (app *App) hostCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "host", Short: "Manage remote hosts"}
	cmd.AddCommand(app.hostAddCmd())
	cmd.AddCommand(app.hostCreateCmd())
	cmd.AddCommand(app.hostListCmd())
	cmd.AddCommand(app.hostUseCmd())
	cmd.AddCommand(app.hostVerifyCmd())
	cmd.AddCommand(app.hostStartCmd())
	cmd.AddCommand(app.hostStopCmd())
	cmd.AddCommand(app.hostRestartCmd())
	cmd.AddCommand(app.hostResizeCmd())
	cmd.AddCommand(app.hostRemoveCmd())
	cmd.AddCommand(app.hostDestroyCmd())
	cmd.AddCommand(app.hostUpdateSSHAccessCmd())
	cmd.AddCommand(app.hostCapabilitiesCmd())
	return cmd
}

func (app *App) hostCreateCmd() *cobra.Command {
	var providerName, region, profile, instanceType, sshCIDR string
	var noCleanup bool
	cmd := &cobra.Command{
		Use:   "create NAME",
		Short: "Provision a new cloud host with at least 20 GiB root storage",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			h, _ := app.Global.ResolveHost(app.HostFlag)
			if h != nil {
				if err := authz.RequireOwner(h, "host create"); err != nil {
					return err
				}
			}
			return (&host.Service{Global: app.Global, Out: app.Out}).Create(context.Background(), host.CreateOpts{
				Name:         args[0],
				ProviderName: providerName,
				Region:       region,
				Profile:      profile,
				InstanceType: instanceType,
				SSHCIDR:      sshCIDR,
				NoCleanup:    noCleanup,
			})
		},
	}
	cmd.Flags().StringVar(&providerName, "provider", "aws", "cloud provider")
	cmd.Flags().StringVar(&region, "region", "", "cloud region")
	cmd.Flags().StringVar(&profile, "profile", "", "AWS profile name")
	cmd.Flags().StringVar(&instanceType, "instance-type", "", "EC2 instance type")
	cmd.Flags().StringVar(&sshCIDR, "ssh-cidr", "", "CIDR allowed for SSH ingress")
	cmd.Flags().BoolVar(&noCleanup, "no-cleanup", false, "do not delete resources on failure")
	return cmd
}

func (app *App) hostStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start NAME",
		Short: "Start a stopped cloud host and wait for SSH",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := (&host.Service{Global: app.Global, Out: app.Out}).Start(context.Background(), args[0]); err != nil {
				return err
			}
			app.Out.Success("Host %q started", args[0])
			return nil
		},
	}
}

func (app *App) hostStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop NAME",
		Short: "Stop a cloud host (pauses compute billing on AWS)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := (&host.Service{Global: app.Global, Out: app.Out}).Stop(context.Background(), args[0]); err != nil {
				return err
			}
			app.Out.Success("Host %q stopped", args[0])
			return nil
		},
	}
}

func (app *App) hostRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart NAME",
		Short: "Restart a cloud host",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := (&host.Service{Global: app.Global, Out: app.Out}).Restart(context.Background(), args[0]); err != nil {
				return err
			}
			app.Out.Success("Host %q restarted", args[0])
			return nil
		},
	}
}

func (app *App) hostUpdateSSHAccessCmd() *cobra.Command {
	var sshCIDR string
	cmd := &cobra.Command{
		Use:   "update-ssh-access NAME",
		Short: "Update SSH ingress on the host security group to your current IP",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return (&host.Service{Global: app.Global, Out: app.Out}).UpdateSSHAccess(context.Background(), host.UpdateSSHAccessOpts{
				Name:    args[0],
				SSHCIDR: sshCIDR,
			})
		},
	}
	cmd.Flags().StringVar(&sshCIDR, "ssh-cidr", "", "CIDR allowed for SSH ingress (default: auto-detect current public IP)")
	return cmd
}

func (app *App) hostResizeCmd() *cobra.Command {
	var instanceType string
	cmd := &cobra.Command{
		Use:   "resize NAME",
		Short: "Resize a cloud host instance type",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if instanceType == "" {
				return fmt.Errorf("--instance-type is required")
			}
			if err := (&host.Service{Global: app.Global, Out: app.Out}).Resize(context.Background(), args[0], instanceType); err != nil {
				return err
			}
			app.Out.Success("Host %q resized to %s", args[0], instanceType)
			return nil
		},
	}
	cmd.Flags().StringVar(&instanceType, "instance-type", "", "new EC2 instance type")
	_ = cmd.MarkFlagRequired("instance-type")
	return cmd
}

func (app *App) providerCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "provider", Short: "Manage cloud provider credentials"}
	cmd.AddCommand(app.providerLoginAWSCmd())
	return cmd
}

func (app *App) providerLoginAWSCmd() *cobra.Command {
	var profile, region string
	cmd := &cobra.Command{
		Use:   "login aws",
		Short: "Validate and store AWS credentials profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			return host.ProviderLogin(context.Background(), app.Global, app.Out, profile, region)
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "AWS shared config profile")
	cmd.Flags().StringVar(&region, "region", "", "default AWS region")
	return cmd
}

func (app *App) hostAddCmd() *cobra.Command {
	var hostname, user, identityFile, password, auth string
	var port int
	var skipBootstrap bool
	cmd := &cobra.Command{
		Use:   "add NAME",
		Short: "Register a remote host after verifying SSH access",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			authMode, err := transport.ParseAuthMode(auth)
			if err != nil {
				return err
			}
			sel, err := transport.ResolveAuthSelection(transport.SSHConfig{
				User:         user,
				Hostname:     hostname,
				AuthMode:     authMode,
				IdentityFile: identityFile,
			}, cmd.Flags().Changed("auth"))
			if err != nil {
				return err
			}
			svc := &host.Service{Global: app.Global, Out: app.Out}
			return svc.Add(context.Background(), host.AddOpts{
				Name:             args[0],
				Hostname:         hostname,
				User:             user,
				Port:             port,
				IdentityFile:     sel.IdentityFile,
				Password:         password,
				AuthMode:         sel.Mode,
				SkipBootstrap:    skipBootstrap,
				AutoTrustHostKey: app.ForceYes,
			})
		},
	}
	cmd.Flags().StringVar(&hostname, "hostname", "", "SSH hostname or IP")
	cmd.Flags().StringVar(&user, "user", "", "SSH user")
	cmd.Flags().IntVar(&port, "port", 22, "SSH port")
	cmd.Flags().StringVar(&auth, "auth", "auto", "SSH auth: auto, password, or key (prompted when omitted)")
	cmd.Flags().StringVar(&identityFile, "identity-file", "", "SSH private key (required for --auth key; optional for auto)")
	cmd.Flags().StringVar(&password, "password", "", "SSH login password (optional; prompted when needed)")
	cmd.Flags().BoolVar(&skipBootstrap, "skip-bootstrap", false, "verify SSH only, without installing Docker")
	_ = cmd.MarkFlagRequired("hostname")
	_ = cmd.MarkFlagRequired("user")
	return cmd
}

func (app *App) hostListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List registered hosts",
		RunE: func(cmd *cobra.Command, args []string) error {
			return (&host.Service{Global: app.Global, Out: app.Out}).List()
		},
	}
}

func (app *App) hostUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use NAME",
		Short: "Set the active host",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := (&host.Service{Global: app.Global, Out: app.Out}).Use(args[0]); err != nil {
				return err
			}
			app.Out.Success("Active host: %s", args[0])
			return nil
		},
	}
}

func (app *App) hostUseAliasCmd() *cobra.Command {
	cmd := app.hostUseCmd()
	cmd.Use = "use NAME"
	return cmd
}

func (app *App) hostVerifyCmd() *cobra.Command {
	var skipBootstrap bool
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify connectivity and bootstrap remote dependencies",
		RunE: func(cmd *cobra.Command, args []string) error {
			return (&host.Service{Global: app.Global, Out: app.Out}).Verify(context.Background(), app.HostFlag, skipBootstrap, app.ForceYes)
		},
	}
	cmd.Flags().BoolVar(&skipBootstrap, "skip-bootstrap", false, "only test SSH connectivity")
	return cmd
}

func (app *App) hostRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove NAME",
		Short: "Remove local host configuration (does not affect the remote server)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := (&host.Service{Global: app.Global, Out: app.Out}).Remove(args[0]); err != nil {
				return err
			}
			app.Out.Success("Removed local configuration for host %q", args[0])
			return nil
		},
	}
}

func (app *App) hostDestroyCmd() *cobra.Command {
	var deleteVolumes bool
	cmd := &cobra.Command{
		Use:   "destroy NAME",
		Short: "Destroy a cloud host (owner only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := (&host.Service{Global: app.Global, Out: app.Out}).Destroy(context.Background(), args[0], deleteVolumes, app.ForceYes); err != nil {
				return err
			}
			app.Out.Success("Destroyed host %q", args[0])
			return nil
		},
	}
	cmd.Flags().BoolVar(&deleteVolumes, "delete-volumes", false, "delete attached EBS volumes")
	return cmd
}

func (app *App) initCmd() *cobra.Command {
	var name, hostName string
	var writeGitignore, noCompose, noShell bool
	openShell := true
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize Outpost for the current repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := project.Init(app.Cwd, name, hostName, writeGitignore, noCompose)
			if err != nil {
				return err
			}
			if app.Out.JSON {
				return app.Out.PrintJSON(p)
			}
			app.Out.Success("Initialized project %q", p.Name)
			app.Out.Info("Remote directory: %s", p.RemoteDir)
			app.Out.Info("Created %s — commit this file so teammates share the same project name and remote path", config.ProjectConfigPath(app.Cwd))
			app.Out.Info("Edit %s to exclude paths from mirror sync", config.OutpostIgnorePath(app.Cwd))
			if !writeGitignore {
				app.Out.Info("Tip: run with --write-gitignore to keep .outpost/ out of git (local-only config)")
			}
			if openShell && !noShell && !app.Out.JSON && mirror.IsInteractiveTerminal() {
				app.Out.Step("")
				app.Out.Step("Opening remote shell — type exit to return")
				return app.withProjectExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project) error {
					return mirror.New(exec, proj, app.Cwd, h.Name, app.Out).Shell(ctx)
				})
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "stable project name")
	cmd.Flags().StringVar(&hostName, "host", "", "host override")
	cmd.Flags().BoolVar(&writeGitignore, "write-gitignore", false, "append .outpost/ to .gitignore")
	cmd.Flags().BoolVar(&noCompose, "no-compose", false, "explicitly initialize without Compose (Compose is optional)")
	cmd.Flags().BoolVar(&noShell, "no-shell", false, "initialize without opening the remote project shell")
	return cmd
}

func (app *App) projectShellCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "shell",
		Short: "Open the remote project development environment",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !app.Out.JSON && mirror.IsInteractiveTerminal() {
				app.Out.Step("Opening remote shell — type exit to return")
			}
			return app.withProjectExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project) error {
				return mirror.New(exec, proj, app.Cwd, h.Name, app.Out).Shell(ctx)
			})
		},
	}
}

func (app *App) projectRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "run [flags] -- COMMAND [ARGS...]",
		Short:              "Run a command in the remote project environment",
		Long: strings.TrimSpace(`
Runs a command on the remote host inside the project environment. By default
Outpost starts a persistent tmux session, attaches your terminal to it, and
keeps the command running if SSH disconnects.

Use --detach to start without attaching, then reconnect with:
  outpost session attach NAME
  outpost session status NAME
  outpost session logs NAME -f

Use --foreground for the legacy SSH-attached behavior (stops when you disconnect).`),
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			flags, cmdArgs, err := mirror.ParseRunCLIArgs(args)
			if err != nil {
				return err
			}
			if flags.Attach != "" {
				return app.withProjectExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project) error {
					result, err := mirror.New(exec, proj, app.Cwd, h.Name, app.Out).Run(ctx, mirror.RunOptions{
						AttachSession: flags.Attach,
					})
					return exitRunResult(app, result, err)
				})
			}
			if len(cmdArgs) == 0 {
				return fmt.Errorf("command is required")
			}
			return app.withProjectExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project) error {
				result, err := mirror.New(exec, proj, app.Cwd, h.Name, app.Out).Run(ctx, mirror.RunOptions{
					Command:     mirror.JoinCommandArgs(cmdArgs),
					Detach:      flags.Detach,
					Foreground:  flags.Foreground,
					SessionName: flags.Name,
				})
				return exitRunResult(app, result, err)
			})
		},
	}
}

func (app *App) projectAICmd() *cobra.Command {
	var (
		command string
		noPull  bool
	)
	cmd := &cobra.Command{
		Use:   "ai [COMMAND]",
		Short: "Run an AI coding agent in the remote project environment",
		Long: strings.TrimSpace(`
Syncs your project to the remote development container, starts an interactive
AI agent there, keeps your local edits flowing up while the session is open,
and pulls agent-made file changes back to your machine when you exit.

By default Outpost tries opencode, then claude, then codex. Override with a
positional command or ai.command in project.yaml.`),
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && args[0] == "--" {
				args = args[1:]
			}
			if command == "" && len(args) > 0 {
				command = strings.Join(args, " ")
			}
			if !app.Out.JSON && mirror.IsInteractiveTerminal() {
				app.Out.Step("Opening remote AI session — exit the agent to return")
			}
			return app.withProjectExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project) error {
				code, err := mirror.New(exec, proj, app.Cwd, h.Name, app.Out).AI(ctx, mirror.AIOptions{
					Command: command,
					NoPull:  noPull,
				})
				if err != nil {
					return err
				}
				if code != 0 {
					os.Exit(code)
				}
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&command, "command", "", "agent command (default: auto-detect opencode, claude, or codex)")
	cmd.Flags().BoolVar(&noPull, "no-pull", false, "skip pulling remote file changes back after the session")
	return cmd
}

func (app *App) projectOpenCmd() *cobra.Command {
	var portSpecs []string
	var localPort int
	var service string
	cmd := &cobra.Command{
		Use:   "open",
		Short: "Forward and display project ports",
		RunE: func(cmd *cobra.Command, args []string) error {
			discover := len(portSpecs) == 0
			return app.connectStart(connectStartOpts{
				service:           service,
				localPortOverride: localPort,
				portSpecs:         portSpecs,
				discover:          discover,
			})
		},
	}
	cmd.Flags().StringArrayVar(&portSpecs, "port", nil, "forward a port mapping (local:remote or remote port only)")
	cmd.Flags().IntVar(&localPort, "local-port", 0, "local port when forwarding a single service")
	cmd.Flags().StringVar(&service, "service", "", "limit discovery to one Compose service")
	return cmd
}

func (app *App) projectCloseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "close",
		Short: "Stop project port forwarding",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.connectDown()
		},
	}
}

func (app *App) cleanupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cleanup",
		Short: "Remove stale Outpost project and Docker artifacts",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.withProjectExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project) error {
				if err := cleanup.Project(ctx, exec, proj, cleanup.OptionsForProject(proj)); err != nil {
					return err
				}
				if err := cleanup.Global(ctx, exec, cleanup.OptionsForProject(proj)); err != nil {
					return err
				}
				app.Out.Success("Outpost cleanup completed")
				return nil
			})
		},
	}
}

func (app *App) migrateCmd() *cobra.Command {
	var fromHost, toHost string
	var dryRun, skipVolumes, skipCluster, skipMachine, skipCompose bool
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate this project's remote environment to another host",
		Long:  "Exports the project Docker environment as a unified bundle (containers, volumes, Kubernetes), plus optional Incus machine and remote .outpost metadata, then restores everything on the destination host.",
		RunE: func(cmd *cobra.Command, args []string) error {
			proj, err := config.LoadProject(app.Cwd)
			if err != nil {
				return err
			}
			_, err = migrate.Run(context.Background(), migrate.Options{
				Cwd:         app.Cwd,
				Project:     proj,
				Global:      app.Global,
				FromHost:    fromHost,
				ToHost:      toHost,
				ForceYes:    app.ForceYes,
				DryRun:      dryRun,
				SkipVolumes: skipVolumes,
				SkipCluster: skipCluster,
				SkipMachine: skipMachine,
				SkipCompose: skipCompose,
				HostFlag:    app.HostFlag,
				Out:         app.Out,
				NewExecutor: app.newExecutor,
			})
			return err
		},
	}
	cmd.Flags().StringVar(&fromHost, "from", "", "source host (default: current project host)")
	cmd.Flags().StringVar(&toHost, "to", "", "destination host")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print migration plan without making changes")
	cmd.Flags().BoolVar(&skipVolumes, "skip-volumes", false, "skip Docker bundle export/import (containers and volumes)")
	cmd.Flags().BoolVar(&skipCluster, "skip-cluster", false, "exclude Kubernetes from the Docker bundle; do not create a cluster on destination")
	cmd.Flags().BoolVar(&skipMachine, "skip-machine", false, "do not create project Incus machine on destination")
	cmd.Flags().BoolVar(&skipCompose, "skip-compose", false, "do not run compose up on destination")
	return cmd
}

func (app *App) dockerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "docker [args...]",
		Short:              "Run docker on the remote host",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.withExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host) error {
				if docker.IsDestructive(args) {
					count, _ := share.ApprovedCount(ctx, exec)
					if err := authz.ConfirmDestructive(count, docker.ActionLabel(args), app.ForceYes); err != nil {
						return err
					}
				}
				code, err := docker.Run(ctx, exec, args)
				if err != nil {
					return err
				}
				if code != 0 {
					os.Exit(code)
				}
				return nil
			})
		},
	}
	return cmd
}

func (app *App) composeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compose",
		Short: "Manage this project's Docker Compose services",
	}
	cmd.AddCommand(app.composeSubCmd("up", true, false))
	cmd.AddCommand(app.composeSubCmd("down", false, true))
	cmd.AddCommand(app.composeSubCmd("ps", false, false))
	cmd.AddCommand(app.composeSubCmd("logs", false, false))
	cmd.AddCommand(app.composeSubCmd("exec", false, false))
	cmd.AddCommand(app.composeSubCmd("build", true, false))
	cmd.AddCommand(app.composeSubCmd("pull", true, false))
	cmd.AddCommand(app.composeVolumesCmd())
	return cmd
}

func (app *App) appCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "app", Short: "Build and run this project's Dockerfile application"}
	cmd.AddCommand(app.appBuildCmd())
	cmd.AddCommand(app.appRunCmd())
	cmd.AddCommand(app.appStopCmd())
	cmd.AddCommand(app.appLogsCmd())
	cmd.AddCommand(app.appStatusCmd())
	return cmd
}

func (app *App) withAppRunner(run func(context.Context, transport.Executor, *config.Host, *config.Project, *appsvc.Runner) error) error {
	return app.withProjectExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project) error {
		return run(ctx, exec, h, proj, &appsvc.Runner{Exec: exec, Project: proj, Out: app.Out})
	})
}

func (app *App) appBuildCmd() *cobra.Command {
	var dockerfile, buildContext string
	cmd := &cobra.Command{Use: "build", Short: "Build the project's Dockerfile application image", RunE: func(cmd *cobra.Command, args []string) error {
		return app.withAppRunner(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project, runner *appsvc.Runner) error {
			if _, err := mirror.New(exec, proj, app.Cwd, h.Name, app.Out).SyncIfNeeded(ctx, false); err != nil {
				return err
			}
			return runner.Build(ctx, dockerfile, buildContext)
		})
	}}
	cmd.Flags().StringVar(&dockerfile, "dockerfile", "Dockerfile", "Dockerfile path relative to the project")
	cmd.Flags().StringVar(&buildContext, "context", ".", "Docker build context relative to the project")
	return cmd
}

func (app *App) appRunCmd() *cobra.Command {
	var ports []string
	var detach, build bool
	cmd := &cobra.Command{Use: "run [--port HOST:CONTAINER] [--detach] [--build] [-- COMMAND...]", Short: "Run the built Dockerfile application", Args: cobra.ArbitraryArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return app.withAppRunner(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project, runner *appsvc.Runner) error {
			if build {
				if _, err := mirror.New(exec, proj, app.Cwd, h.Name, app.Out).SyncIfNeeded(ctx, false); err != nil {
					return err
				}
				if err := runner.Build(ctx, "Dockerfile", "."); err != nil {
					return err
				}
			}
			code, err := runner.Run(ctx, ports, detach, args)
			if err != nil {
				return err
			}
			if code != 0 {
				os.Exit(code)
			}
			return nil
		})
	}}
	cmd.Flags().StringArrayVar(&ports, "port", nil, "publish a port (HOST:CONTAINER)")
	cmd.Flags().BoolVarP(&detach, "detach", "d", false, "run in the background")
	cmd.Flags().BoolVar(&build, "build", false, "build the image before running")
	return cmd
}

func (app *App) appStopCmd() *cobra.Command {
	return &cobra.Command{Use: "stop", Short: "Stop the project application container", RunE: func(cmd *cobra.Command, args []string) error {
		return app.withAppRunner(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project, runner *appsvc.Runner) error {
			return runner.Stop(ctx)
		})
	}}
}

func (app *App) appLogsCmd() *cobra.Command {
	var follow bool
	cmd := &cobra.Command{Use: "logs", Short: "Show project application logs", RunE: func(cmd *cobra.Command, args []string) error {
		return app.withAppRunner(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project, runner *appsvc.Runner) error {
			return runner.Logs(ctx, follow)
		})
	}}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow logs")
	return cmd
}

func (app *App) appStatusCmd() *cobra.Command {
	return &cobra.Command{Use: "status", Short: "Show the project application status", RunE: func(cmd *cobra.Command, args []string) error {
		return app.withAppRunner(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project, runner *appsvc.Runner) error {
			status, err := runner.Status(ctx)
			if err != nil {
				return err
			}
			if app.Out.JSON {
				return app.Out.PrintJSON(map[string]string{"status": strings.TrimSpace(status), "image": runner.Image(), "container": runner.Container()})
			}
			app.Out.Info("Application: %s", strings.TrimSpace(status))
			return nil
		})
	}}
}

func (app *App) composeVolumesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "volumes",
		Short: "Export and import Docker Compose named volumes",
	}
	cmd.AddCommand(app.composeVolumesListCmd())
	cmd.AddCommand(app.composeVolumesExportCmd())
	cmd.AddCommand(app.composeVolumesImportCmd())
	return cmd
}

func (app *App) composeVolumesListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List compose volumes and local archive status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.withProjectExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project) error {
				status, err := compose.ListVolumeStatus(ctx, exec, app.Cwd, proj)
				if err != nil {
					return err
				}
				if app.Out.JSON {
					return app.Out.PrintJSON(status)
				}
				if len(status) == 0 {
					app.Out.Info("No named compose volumes found")
					return nil
				}
				for _, st := range status {
					hostState := "missing"
					if st.OnHost {
						if st.EmptyOnHost {
							hostState = "empty"
						} else {
							hostState = "present"
						}
					}
					archiveState := "missing"
					if st.HasArchive {
						archiveState = fmt.Sprintf("present (%s)", formatBytes(uint64(st.ArchiveBytes)))
					}
					app.Out.Info("%s (%s): host=%s archive=%s", st.LogicalName, st.DockerName, hostState, archiveState)
				}
				if proj.Volumes != nil && proj.Volumes.LastHost != "" {
					app.Out.Info("Last volume sync host: %s", proj.Volumes.LastHost)
				}
				return nil
			})
		},
	}
}

func (app *App) composeVolumesExportCmd() *cobra.Command {
	var volumeName string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export compose volumes from the remote host to local archives",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.withProjectExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project) error {
				if err := compose.ExportVolumes(ctx, exec, app.Cwd, proj, h.Name, compose.VolumeOptions{
					VolumeName: volumeName,
				}); err != nil {
					return err
				}
				app.Out.Success("Compose volumes exported to local archives")
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&volumeName, "volume", "", "export a single volume by logical name")
	return cmd
}

func (app *App) composeVolumesImportCmd() *cobra.Command {
	var volumeName string
	var force bool
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import compose volumes from local archives to the remote host",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.withProjectExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project) error {
				if err := compose.ImportVolumes(ctx, exec, app.Cwd, proj, h.Name, compose.VolumeOptions{
					VolumeName: volumeName,
					Force:      force,
					ForceYes:   app.ForceYes,
				}); err != nil {
					return err
				}
				app.Out.Success("Compose volumes imported to remote host")
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&volumeName, "volume", "", "import a single volume by logical name")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing remote volumes")
	return cmd
}

func (app *App) composeSubCmd(sub string, uploadFirst, checkDestructive bool) *cobra.Command {
	return &cobra.Command{
		Use:                sub,
		Short:              fmt.Sprintf("docker compose %s on the remote host", sub),
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.withProjectExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project) error {
				if checkDestructive && compose.IsDestructive(sub, args) {
					count, _ := share.ApprovedCount(ctx, exec)
					if err := authz.ConfirmDestructive(count, "compose "+sub, app.ForceYes); err != nil {
						return err
					}
				}
				runner := &compose.Runner{
					Exec:     exec,
					Project:  proj,
					Cwd:      app.Cwd,
					HostName: h.Name,
					ForceYes: app.ForceYes,
					Out:      app.Out,
				}
				code, err := runner.Run(ctx, sub, args, uploadFirst)
				if err != nil {
					return err
				}
				if code != 0 {
					os.Exit(code)
				}
				return nil
			})
		},
	}
}

type connectStartOpts struct {
	service           string
	localPortOverride int
	portSpecs         []string
	foreground        bool
	discover          bool
}

func (app *App) connectStart(opts connectStartOpts) error {
	if !opts.foreground && !connect.IsWorker() {
		pid, err := connect.SpawnDetached(os.Args, []string{connect.WorkerEnvKey + "=1"})
		if err != nil {
			return err
		}
		proj, err := config.LoadProject(app.Cwd)
		if err != nil {
			return err
		}
		hostName := app.resolveHostName(proj)
		sess, err := connect.WaitForSession(hostName, proj.Name, 60*time.Second)
		if err != nil {
			return fmt.Errorf("started background forwarder (pid %d) but session was not ready: %w", pid, err)
		}
		for _, f := range sess.Forwards {
			if f.Service == "kubernetes" {
				app.Out.Success("Kubernetes API tunnel ready on 127.0.0.1:%d", f.LocalPort)
				continue
			}
			app.Out.Success("%s (service: %s)", f.URL, f.Service)
		}
		app.Out.Info("Forwarding running in background (pid %d). Stop with: outpost close", pid)
		return nil
	}

	return app.withProjectExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project) error {
		hasKubernetes := proj.Kubernetes != nil
		var kubeconfig []byte
		var kubeRemotePort, kubeLocalPort int
		if hasKubernetes {
			projectCluster := cluster.NewProjectService(exec, proj, app.Cwd, app.Out)
			var err error
			kubeconfig, err = projectCluster.Kubeconfig()
			if err != nil {
				return fmt.Errorf("project Kubernetes cluster is not ready — run 'outpost cluster up' first: %w", err)
			}
			kubeRemotePort, err = projectCluster.APIPort(kubeconfig)
			if err != nil {
				return err
			}
			kubeLocalPort, err = connect.AvailablePort("127.0.0.1", kubeRemotePort)
			if err != nil {
				return err
			}
		}
		if len(opts.portSpecs) == 0 && !hasKubernetes {
			if err := proj.RequireCompose(); err != nil && !(proj.EnvironmentEnabled() && proj.Environment != nil && len(proj.Environment.Ports) > 0) {
				return err
			}
		}
		if err := connect.EnsureNoActiveSession(h.Name, proj.Name); err != nil {
			return err
		}
		if hasKubernetes {
			_ = os.Remove(cluster.LocalProjectKubeconfigPath(app.Cwd))
		}
		composeArgs := upload.RemoteComposeArgs(proj)
		var mappings []connect.PortMapping
		var err error
		if len(opts.portSpecs) > 0 || opts.discover || proj.RequireCompose() == nil || (proj.EnvironmentEnabled() && proj.Environment != nil && len(proj.Environment.Ports) > 0) {
			mappings, err = connect.ResolvePortMappings(ctx, exec, app.Cwd, proj, composeArgs, connect.ResolveOptions{
				Service:     opts.service,
				Discover:    opts.discover,
				ManualSpecs: opts.portSpecs,
			})
			if err != nil {
				if !hasKubernetes {
					return err
				}
				mappings = nil
			}
		} else if !hasKubernetes {
			return fmt.Errorf("no application ports to forward")
		}
		overrides := map[string]int{}
		if opts.localPortOverride > 0 && len(mappings) == 1 {
			overrides[mappings[0].Service] = opts.localPortOverride
		}
		if hasKubernetes {
			for {
				conflict := false
				for _, mapping := range mappings {
					localPort := mapping.HostPort
					if override, ok := overrides[mapping.Service]; ok {
						localPort = override
					}
					if localPort == kubeLocalPort {
						conflict = true
						break
					}
				}
				if !conflict {
					break
				}
				kubeLocalPort, err = connect.AvailablePort("127.0.0.1", 0)
				if err != nil {
					return err
				}
			}
			mappings = append(mappings, connect.PortMapping{Service: "kubernetes", HostPort: kubeRemotePort, TargetPort: kubeRemotePort, BindHost: "127.0.0.1"})
		}
		if hasKubernetes {
			overrides["kubernetes"] = kubeLocalPort
		}
		forwards, closers, err := connect.StartForwards(ctx, exec, mappings, overrides)
		if err != nil {
			return err
		}
		if hasKubernetes {
			for _, f := range forwards {
				if f.Service != "kubernetes" {
					continue
				}
				rewritten, rewriteErr := cluster.RewriteKubeconfigServer(kubeconfig, f.LocalPort)
				if rewriteErr != nil {
					for _, c := range closers {
						c.Close()
					}
					return rewriteErr
				}
				if mkdirErr := os.MkdirAll(filepath.Dir(cluster.LocalProjectKubeconfigPath(app.Cwd)), 0700); mkdirErr != nil {
					for _, c := range closers {
						c.Close()
					}
					return mkdirErr
				}
				if writeErr := os.WriteFile(cluster.LocalProjectKubeconfigPath(app.Cwd), rewritten, 0600); writeErr != nil {
					for _, c := range closers {
						c.Close()
					}
					return writeErr
				}
				break
			}
		}
		sess := &connect.Session{
			Host:      h.Name,
			Project:   proj.Name,
			PID:       os.Getpid(),
			StartedAt: time.Now().UTC(),
			Forwards:  forwards,
		}
		if err := connect.SaveSession(sess); err != nil {
			for _, c := range closers {
				c.Close()
			}
			return err
		}
		for _, f := range forwards {
			if f.Service == "kubernetes" {
				app.Out.Success("Kubernetes API tunnel ready on 127.0.0.1:%d", f.LocalPort)
				continue
			}
			app.Out.Success("%s (service: %s)", f.URL, f.Service)
		}
		if opts.foreground {
			app.Out.Info("Press Ctrl+C to stop forwarding")
		} else {
			app.Out.Info("Forwarding running in background (pid %d). Stop with: outpost close", os.Getpid())
		}
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		for _, c := range closers {
			c.Close()
		}
		if hasKubernetes {
			_ = os.Remove(cluster.LocalProjectKubeconfigPath(app.Cwd))
		}
		_ = connect.RemoveSession(h.Name, proj.Name)
		return nil
	})
}
func (app *App) connectDown() error {
	proj, err := config.LoadProject(app.Cwd)
	if err != nil {
		return err
	}
	hostName := app.resolveHostName(proj)
	if err := connect.StopSession(hostName, proj.Name); err != nil {
		return err
	}
	if proj.Kubernetes != nil {
		_ = os.Remove(cluster.LocalProjectKubeconfigPath(app.Cwd))
	}
	app.Out.Success("Forwarding session stopped")
	return nil
}
func (app *App) inviteCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "invite", Short: "Manage host sharing invitations"}
	cmd.AddCommand(app.inviteCreateCmd())
	cmd.AddCommand(app.inviteJoinCmd())
	cmd.AddCommand(app.inviteListCmd())
	cmd.AddCommand(app.inviteApproveCmd())
	cmd.AddCommand(app.inviteRevokeCmd())
	return cmd
}

func (app *App) inviteCreateCmd() *cobra.Command {
	var ttl string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an invitation code",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := time.ParseDuration(ttl)
			if err != nil {
				return fmt.Errorf("invalid --ttl: %w", err)
			}
			return app.withExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host) error {
				if err := authz.RequireOwner(h, "invite create"); err != nil {
					return err
				}
				svc := &share.Service{Global: app.Global, Exec: exec, Host: h}
				code, err := svc.CreateInvitation(ctx, d)
				if err != nil {
					return err
				}
				if app.Out.JSON {
					return app.Out.PrintJSON(map[string]string{"code": code})
				}
				app.Out.Success("Invitation code: %s", code)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&ttl, "ttl", "72h", "invitation lifetime")
	return cmd
}

func (app *App) inviteJoinCmd() *cobra.Command {
	var label, hostname, user, identityFile, joinHost string
	var port int
	cmd := &cobra.Command{
		Use:   "join CODE",
		Short: "Join a host using an invitation code",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if label == "" {
				label, _ = os.Hostname()
			}
			return app.inviteJoin(args[0], label, joinHost, hostname, user, port, identityFile)
		},
	}
	cmd.Flags().StringVar(&label, "label", "", "device label")
	cmd.Flags().StringVar(&joinHost, "host", "", "registered host used to reach the server for registration")
	cmd.Flags().StringVar(&hostname, "hostname", "", "remote hostname (required when --host is not set)")
	cmd.Flags().StringVar(&user, "user", "", "SSH user (required when --host is not set)")
	cmd.Flags().IntVar(&port, "port", 22, "SSH port")
	cmd.Flags().StringVar(&identityFile, "identity-file", "", "SSH identity file for registration")
	return cmd
}

func (app *App) inviteJoin(code, label, joinHost, hostname, user string, port int, identityFile string) error {
	ctx := context.Background()
	var exec transport.Executor
	var h *config.Host
	var close func()

	if joinHost != "" {
		var err error
		exec, h, err = app.newExecutor(joinHost)
		if err != nil {
			return err
		}
		if c, ok := exec.(*transport.SSHExecutor); ok {
			close = func() { c.Close() }
		}
		if hostname == "" {
			hostname = h.Hostname
		}
		if user == "" {
			user = h.User
		}
		if strings.TrimSpace(hostname) == "" || strings.TrimSpace(user) == "" {
			return fmt.Errorf("host %q is missing connection details — pass --hostname and --user, or use a registered host that has them", joinHost)
		}
	} else {
		if hostname == "" || user == "" {
			return fmt.Errorf("join requires --hostname and --user, or an existing --host entry for registration SSH")
		}
		sshExec, err := transport.NewSSH(transport.SSHConfig{
			Hostname:         hostname,
			User:             user,
			Port:             port,
			IdentityFile:     config.ExpandPath(identityFile),
			PromptAuth:       true,
			AutoTrustHostKey: app.ForceYes,
		})
		if err != nil {
			return err
		}
		exec = sshExec
		close = func() { sshExec.Close() }
		h = &config.Host{Name: "join", Hostname: hostname, User: user, Port: port, Role: config.RoleMember}
	}
	if close != nil {
		defer close()
	}
	svc := &share.Service{Global: app.Global, Exec: exec, Host: h}
	if err := svc.JoinInvitation(ctx, code, label, hostname, user, port); err != nil {
		return err
	}
	app.Out.Success("Join request submitted for device %q (pending owner approval)", label)
	return nil
}

func (app *App) inviteListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List invitations and devices",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.withExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host) error {
				if err := authz.RequireOwner(h, "invite list"); err != nil {
					return err
				}
				svc := &share.Service{Global: app.Global, Exec: exec, Host: h}
				m, err := svc.List(ctx)
				if err != nil {
					return err
				}
				if app.Out.JSON {
					return app.Out.PrintJSON(m)
				}
				for _, inv := range m.Invitations {
					app.Out.Info("invitation %s expires %s", inv.Code, inv.ExpiresAt.Format(time.RFC3339))
				}
				for _, d := range m.Devices {
					app.Out.Info("device %s %s status=%s", d.ID[:8], d.Label, d.Status)
				}
				return nil
			})
		},
	}
}

func (app *App) inviteApproveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "approve DEVICE_ID",
		Short: "Approve a pending device",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.withExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host) error {
				if err := authz.RequireOwner(h, "invite approve"); err != nil {
					return err
				}
				svc := &share.Service{Global: app.Global, Exec: exec, Host: h}
				if err := svc.Approve(ctx, args[0]); err != nil {
					return err
				}
				app.Out.Success("Device approved")
				return nil
			})
		},
	}
}

func (app *App) inviteRevokeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "revoke DEVICE_ID",
		Short: "Revoke device access",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.withExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host) error {
				if err := authz.RequireOwner(h, "invite revoke"); err != nil {
					return err
				}
				svc := &share.Service{Global: app.Global, Exec: exec, Host: h}
				if err := svc.Revoke(ctx, args[0]); err != nil {
					return err
				}
				app.Out.Success("Device revoked")
				return nil
			})
		},
	}
}

func (app *App) hostCapabilitiesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "capabilities",
		Short: "Report host runtime capabilities",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.withExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host) error {
				report, err := capabilities.DetectWithProvider(ctx, exec, h.Provider)
				if err != nil {
					return err
				}
				if app.Out.JSON {
					return app.Out.PrintJSON(report)
				}
				for _, c := range report.Supported {
					app.Out.Info("%s: %s", c.Name, c.Status)
				}
				for _, c := range report.Unavailable {
					app.Out.Info("%s: %s (%s)", c.Name, c.Status, c.Reason)
				}
				return nil
			})
		},
	}
}

func (app *App) statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show host and workload status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.withExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host) error {
				report, err := status.Collect(ctx, exec)
				if err != nil {
					return err
				}
				if app.Out.JSON {
					return app.Out.PrintJSON(report)
				}
				app.Out.Info("Host %s: %d CPU cores (%.1f%%), memory %s/%s, disk %s/%s, uptime %s",
					h.Name, report.Host.CPUCores, report.Host.CPUUsagePercent,
					formatBytes(report.Host.MemoryUsed), formatBytes(report.Host.MemoryTotal),
					formatBytes(report.Host.DiskUsed), formatBytes(report.Host.DiskTotal),
					formatDuration(report.Host.UptimeSeconds))
				if report.Docker.Healthy {
					app.Out.Info("Docker: healthy — %d running, %d stopped containers",
						report.Docker.ContainersRun, report.Docker.ContainersStop)
				} else {
					app.Out.Info("Docker: unavailable")
				}
				if proj, err := config.LoadProject(app.Cwd); err == nil && proj.EnvironmentEnabled() {
					state, _ := environment.New(exec, proj, app.Cwd).Status(ctx)
					if state == "" {
						state = "not created"
					}
					app.Out.Info("Project container %s: %s", proj.Name, state)
				}
				for _, p := range report.Compose {
					app.Out.Info("Compose project %s: %s", p.Name, p.Status)
				}
				if report.Sharing.ApprovedDevices > 0 {
					app.Out.Info("Sharing: %d approved device(s)", report.Sharing.ApprovedDevices)
				}
				if report.Clusters > 0 {
					app.Out.Info("Kubernetes clusters: %d", report.Clusters)
				}
				if report.Machines > 0 {
					app.Out.Info("Machines: %d", report.Machines)
				}
				return nil
			})
		},
	}
}

func (app *App) topCmd() *cobra.Command {
	var watch bool
	cmd := &cobra.Command{
		Use:   "top",
		Short: "Show live container resource usage",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.withExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host) error {
				if watch {
					return top.RunWatch(ctx, exec, app.Out.Stdout, 2*time.Second)
				}
				return top.RunOnce(ctx, exec, app.Out.Stdout)
			})
		},
	}
	cmd.Flags().BoolVarP(&watch, "watch", "w", false, "refresh continuously")
	return cmd
}

func (app *App) capacityCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "capacity",
		Short: "Show available host capacity and recommendations",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.withExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host) error {
				report, err := capacity.Collect(ctx, exec)
				if err != nil {
					return err
				}
				if app.Out.JSON {
					return app.Out.PrintJSON(report)
				}
				app.Out.Info("Available: %.1f CPU cores, %s memory, %s disk",
					report.AvailableCPU, formatBytes(report.AvailableMem), formatBytes(report.AvailableDisk))
				app.Out.Info("%s", report.Recommendation)
				return nil
			})
		},
	}
}
func (app *App) machineCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "machine", Short: "Manage this project's Incus machine"}
	cmd.AddCommand(app.projectMachineUpCmd())
	cmd.AddCommand(app.projectMachineDownCmd())
	cmd.AddCommand(app.projectMachineStatusCmd())
	cmd.AddCommand(app.projectMachineShellCmd())
	cmd.AddCommand(app.projectMachineExecCmd())
	cmd.AddCommand(app.projectMachineCopyCmd())
	cmd.AddCommand(app.projectMachineConnectCmd())
	cmd.AddCommand(app.projectMachineSnapshotCmd())
	return cmd
}
func (app *App) withProjectMachineExecutor(run func(context.Context, transport.Executor, *config.Host, *config.Project, *machine.ProjectService) error) error {
	return app.withProjectExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project) error {
		if h.Role != config.RoleMember {
			if err := bootstrap.EnsureIncusWithOut(ctx, exec, app.Out); err != nil {
				return err
			}
		}
		return run(ctx, exec, h, proj, machine.NewProjectService(exec, proj, app.Out))
	})
}

func (app *App) projectMachineUpCmd() *cobra.Command {
	var image, memory, disk string
	var cpu float64
	var virtualMachine bool
	cmd := &cobra.Command{
		Use:   "up",
		Short: "Create or reuse this project's Incus machine",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.withProjectMachineExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project, svc *machine.ProjectService) error {
				opts := machine.CreateOptions{}
				if proj.Machine != nil {
					opts.Image, opts.CPU, opts.VirtualMachine = proj.Machine.Image, proj.Machine.CPU, proj.Machine.VirtualMachine
					if proj.Machine.Memory != "" {
						v, err := machine.ParseSize(proj.Machine.Memory)
						if err != nil {
							return err
						}
						opts.MemoryBytes = v
					}
					if proj.Machine.Disk != "" {
						v, err := machine.ParseSize(proj.Machine.Disk)
						if err != nil {
							return err
						}
						opts.DiskBytes = v
					}
				}
				if cmd.Flags().Changed("image") {
					opts.Image = image
				}
				if cmd.Flags().Changed("cpu") {
					opts.CPU = cpu
				}
				if cmd.Flags().Changed("memory") {
					v, err := machine.ParseSize(memory)
					if err != nil {
						return err
					}
					opts.MemoryBytes = v
				}
				if cmd.Flags().Changed("disk") {
					v, err := machine.ParseSize(disk)
					if err != nil {
						return err
					}
					opts.DiskBytes = v
				}
				if cmd.Flags().Changed("virtual-machine") {
					opts.VirtualMachine = virtualMachine
				}
				if err := svc.Up(ctx, opts, h.Provider); err != nil {
					return err
				}
				opts.ApplyDefaults()
				proj.Machine = &config.ProjectMachine{Image: opts.Image, CPU: opts.CPU, Memory: machine.FormatSize(opts.MemoryBytes), Disk: machine.FormatSize(opts.DiskBytes), VirtualMachine: opts.VirtualMachine}
				return config.SaveProject(app.Cwd, proj)
			})
		},
	}
	cmd.Flags().StringVar(&image, "image", "", "Incus image alias")
	cmd.Flags().Float64Var(&cpu, "cpu", 0, "CPU cores")
	cmd.Flags().StringVar(&memory, "memory", "", "memory limit (e.g. 2GiB)")
	cmd.Flags().StringVar(&disk, "disk", "", "root disk size (e.g. 20GiB)")
	cmd.Flags().BoolVar(&virtualMachine, "virtual-machine", false, "create a hardware-virtualized VM")
	return cmd
}

func (app *App) projectMachineDownCmd() *cobra.Command {
	return &cobra.Command{Use: "down", Short: "Delete this project's Incus machine", RunE: func(cmd *cobra.Command, args []string) error {
		return app.withProjectMachineExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project, svc *machine.ProjectService) error {
			if err := authz.RequireOwner(h, "machine down"); err != nil {
				return err
			}
			count, _ := share.ApprovedCount(ctx, exec)
			if err := authz.ConfirmDestructive(count, "machine down", app.ForceYes); err != nil {
				return err
			}
			if !app.ForceYes {
				if err := authz.ConfirmPrompt("This will permanently delete the project's Incus machine"); err != nil {
					return err
				}
			}
			if err := svc.Down(ctx); err != nil {
				return err
			}
			if err := config.SaveProject(app.Cwd, proj); err != nil {
				return err
			}
			app.Out.Success("Deleted project machine")
			return nil
		})
	}}
}

func (app *App) projectMachineStatusCmd() *cobra.Command {
	return &cobra.Command{Use: "status", Short: "Show this project's machine status", RunE: func(cmd *cobra.Command, args []string) error {
		return app.withProjectMachineExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project, svc *machine.ProjectService) error {
			m, err := svc.Status(ctx)
			if err != nil {
				return err
			}
			if app.Out.JSON {
				return app.Out.PrintJSON(m)
			}
			app.Out.Info("Project machine: type=%s (%s) status=%s image=%s ip=%s", m.Type, machine.TypeLabel(m.Type), m.Status, m.Image, m.IPv4)
			return nil
		})
	}}
}

func (app *App) projectMachineShellCmd() *cobra.Command {
	return &cobra.Command{Use: "shell", Short: "Open a shell in this project's machine", RunE: func(cmd *cobra.Command, args []string) error {
		return app.withProjectMachineExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project, svc *machine.ProjectService) error {
			return svc.Shell(ctx)
		})
	}}
}

func (app *App) projectMachineExecCmd() *cobra.Command {
	return &cobra.Command{Use: "exec -- COMMAND [ARGS...]", Short: "Run a command in this project's machine", DisableFlagParsing: true, RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 && args[0] == "--" {
			args = args[1:]
		}
		if len(args) == 0 {
			return fmt.Errorf("command is required")
		}
		return app.withProjectMachineExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project, svc *machine.ProjectService) error {
			code, err := svc.RunCommand(ctx, args)
			if err != nil {
				return err
			}
			if code != 0 {
				os.Exit(code)
			}
			return nil
		})
	}}
}

func (app *App) projectMachineCopyCmd() *cobra.Command {
	var recursive bool
	cmd := &cobra.Command{Use: "copy SRC DST", Short: "Copy files between your computer and this project's machine", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		return app.withProjectMachineExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project, svc *machine.ProjectService) error {
			return svc.Copy(ctx, args[0], args[1], recursive)
		})
	}}
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "recursively copy directories")
	return cmd
}

func (app *App) projectMachineConnectCmd() *cobra.Command {
	var ports []string
	var bind string
	cmd := &cobra.Command{Use: "connect", Short: "Forward ports from this project's machine", RunE: func(cmd *cobra.Command, args []string) error {
		return app.withProjectMachineExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project, svc *machine.ProjectService) error {
			forwards, closers, err := svc.StartConnect(ctx, ports, bind)
			if err != nil {
				return err
			}
			defer func() {
				for _, c := range closers {
					c.Close()
				}
			}()
			for _, f := range forwards {
				app.Out.Success("%s -> machine port %d", f.URL, f.RemotePort)
			}
			app.Out.Info("Press Ctrl+C to stop forwarding")
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			<-sigCh
			return nil
		})
	}}
	cmd.Flags().StringArrayVar(&ports, "port", nil, "port mapping (local:remote or just remote)")
	cmd.Flags().StringVar(&bind, "bind", "127.0.0.1", "local address to bind")
	return cmd
}

func (app *App) projectMachineSnapshotCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "snapshot", Short: "Manage this project's machine snapshots"}
	cmd.AddCommand(&cobra.Command{Use: "create [SNAPSHOT]", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		name := ""
		if len(args) > 0 {
			name = args[0]
		}
		return app.withProjectMachineExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project, svc *machine.ProjectService) error {
			return svc.SnapshotCreate(ctx, name)
		})
	}})
	cmd.AddCommand(&cobra.Command{Use: "list", RunE: func(cmd *cobra.Command, args []string) error {
		return app.withProjectMachineExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project, svc *machine.ProjectService) error {
			v, err := svc.SnapshotList(ctx)
			if err != nil {
				return err
			}
			if app.Out.JSON {
				return app.Out.PrintJSON(v)
			}
			for _, x := range v {
				app.Out.Info("%s", x)
			}
			return nil
		})
	}})
	cmd.AddCommand(&cobra.Command{Use: "delete SNAPSHOT", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return app.withProjectMachineExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project, svc *machine.ProjectService) error {
			if err := authz.RequireOwner(h, "machine snapshot delete"); err != nil {
				return err
			}
			return svc.SnapshotDelete(ctx, args[0])
		})
	}})
	return cmd
}
func (app *App) clusterCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "cluster", Short: "Manage this project's Kubernetes cluster"}
	cmd.AddCommand(app.projectClusterUpCmd())
	cmd.AddCommand(app.projectClusterDownCmd())
	cmd.AddCommand(app.projectClusterEnvCmd())
	cmd.AddCommand(app.clusterStatusCmd())
	return cmd
}

func (app *App) projectClusterUpCmd() *cobra.Command {
	var driver string
	cmd := &cobra.Command{
		Use:   "up",
		Short: "Create or reuse this project's Kubernetes cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.withProjectExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project) error {
				if _, err := mirror.New(exec, proj, app.Cwd, h.Name, app.Out).SyncIfNeeded(ctx, false); err != nil {
					return err
				}
				svc := cluster.NewProjectService(exec, proj, app.Cwd, app.Out)
				return svc.Up(ctx, driver)
			})
		},
	}
	cmd.Flags().StringVar(&driver, "driver", "", "Kubernetes runtime driver (kind or k3d; default kind)")
	return cmd
}

func (app *App) projectClusterDownCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "down",
		Short: "Delete this project's Kubernetes cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.withProjectExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project) error {
				if err := authz.RequireOwner(h, "cluster down"); err != nil {
					return err
				}
				count, _ := share.ApprovedCount(ctx, exec)
				if err := authz.ConfirmDestructive(count, "cluster down", app.ForceYes); err != nil {
					return err
				}
				if !app.ForceYes {
					if err := authz.ConfirmPrompt("This will delete the project's Kubernetes cluster and its node containers"); err != nil {
						return err
					}
				}
				if _, err := connect.LoadActiveSession(h.Name, proj.Name); err == nil {
					if err := connect.StopSession(h.Name, proj.Name); err != nil {
						return err
					}
				}
				if err := cluster.NewProjectService(exec, proj, app.Cwd, app.Out).Down(ctx); err != nil {
					return err
				}
				app.Out.Success("Deleted project Kubernetes cluster")
				return nil
			})
		},
	}
}

func (app *App) projectClusterEnvCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "env -- COMMAND [ARGS...]",
		Short:              "Run a local command with this project's Kubernetes config",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && args[0] == "--" {
				args = args[1:]
			}
			if len(args) == 0 {
				return fmt.Errorf("command is required")
			}
			proj, err := config.LoadProject(app.Cwd)
			if err != nil {
				return err
			}
			path := cluster.LocalProjectKubeconfigPath(app.Cwd)
			if _, err := os.Stat(path); err != nil {
				return fmt.Errorf("project kubeconfig is unavailable — run 'outpost open' first")
			}
			hostName := app.resolveHostName(proj)
			sess, err := connect.LoadActiveSession(hostName, proj.Name)
			if err != nil {
				return fmt.Errorf("Kubernetes tunnel is not active — run 'outpost open' first")
			}
			apiForward := false
			for _, forward := range sess.Forwards {
				if forward.Service == "kubernetes" {
					apiForward = true
					break
				}
			}
			if !apiForward {
				return fmt.Errorf("Kubernetes tunnel is not active — run 'outpost open' first")
			}
			local := osexec.Command(args[0], args[1:]...)
			local.Env = make([]string, 0, len(os.Environ())+1)
			for _, value := range os.Environ() {
				if !strings.HasPrefix(value, "KUBECONFIG=") {
					local.Env = append(local.Env, value)
				}
			}
			local.Env = append(local.Env, "KUBECONFIG="+path)
			local.Stdin, local.Stdout, local.Stderr = os.Stdin, os.Stdout, os.Stderr
			if err := local.Run(); err != nil {
				if exitErr, ok := err.(*osexec.ExitError); ok {
					os.Exit(exitErr.ExitCode())
				}
				return err
			}
			return nil
		},
	}
	return cmd
}
func (app *App) clusterStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show this project's Kubernetes status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.withProjectExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project) error {
				c, err := cluster.NewProjectService(exec, proj, app.Cwd, app.Out).Status(ctx)
				if err != nil {
					return err
				}
				if app.Out.JSON {
					return app.Out.PrintJSON(c)
				}
				app.Out.Info("Project Kubernetes: driver=%s status=%s nodes=%d", c.Driver, c.Status, c.NodeCount)
				return nil
			})
		},
	}
}
func (app *App) diskCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disk",
		Short: "Show disk usage and reclaimable space",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.withExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host) error {
				report, err := disk.Collect(ctx, exec)
				if err != nil {
					return err
				}
				if app.Out.JSON {
					return app.Out.PrintJSON(report)
				}
				app.Out.Info("Filesystem: %s used / %s (%.1f%%)",
					formatBytes(report.Filesystem.UsedBytes), formatBytes(report.Filesystem.TotalBytes),
					report.Filesystem.UsedPercent)
				for _, row := range report.Docker.DiskUsage {
					app.Out.Info("Docker %s: %s (reclaimable %s)", row.Type, row.Size, row.Reclaimable)
				}
				app.Out.Info("Outpost projects: %s", formatBytes(report.Outpost.ProjectsBytes))
				if report.Outpost.ToolchainsBytes > 0 {
					app.Out.Info("Outpost toolchains: %s", formatBytes(report.Outpost.ToolchainsBytes))
				}
				if report.Outpost.ClustersBytes > 0 {
					app.Out.Info("Outpost clusters: %s", formatBytes(report.Outpost.ClustersBytes))
				}
				if report.Outpost.MachinesBytes > 0 {
					app.Out.Info("Outpost machines: %s", formatBytes(report.Outpost.MachinesBytes))
				}
				app.Out.Info("Reclaimable: %d stopped containers, %d dangling images, %d unused networks",
					report.Reclaimable.StoppedContainers, report.Reclaimable.DanglingImages,
					report.Reclaimable.UnusedNetworks)
				if report.Reclaimable.UploadArtifacts != "" {
					app.Out.Info("Upload artifacts: %s", report.Reclaimable.UploadArtifacts)
				}
				return nil
			})
		},
	}
}

func (app *App) pruneCmd() *cobra.Command {
	var dryRun, volumes, force bool
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Safely reclaim disk space on the remote host",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.runPrune(dryRun, volumes, force)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "list cleanup candidates without making changes")
	cmd.Flags().BoolVar(&force, "force", false, "skip volume name confirmation when used with --yes")
	volumesCmd := &cobra.Command{
		Use:   "volumes",
		Short: "Prune unused named volumes (explicit)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.runPrune(dryRun, true, force)
		},
	}
	volumesCmd.Flags().BoolVar(&dryRun, "dry-run", false, "list cleanup candidates without making changes")
	volumesCmd.Flags().BoolVar(&force, "force", false, "skip volume name confirmation when used with --yes")
	cmd.AddCommand(volumesCmd)
	clustersCmd := &cobra.Command{
		Use:   "clusters",
		Short: "Prune Kubernetes clusters (kind and k3d, explicit, owner only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.runPruneClusters(dryRun, force)
		},
	}
	clustersCmd.Flags().BoolVar(&dryRun, "dry-run", false, "list cleanup candidates without making changes")
	clustersCmd.Flags().BoolVar(&force, "force", false, "skip confirmation when used with --yes")
	cmd.AddCommand(clustersCmd)
	machinesCmd := &cobra.Command{
		Use:   "machines",
		Short: "Prune stopped Incus machines (explicit, owner only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.runPruneMachines(dryRun, force)
		},
	}
	machinesCmd.Flags().BoolVar(&dryRun, "dry-run", false, "list cleanup candidates without making changes")
	machinesCmd.Flags().BoolVar(&force, "force", false, "skip confirmation when used with --yes")
	cmd.AddCommand(machinesCmd)
	return cmd
}

func (app *App) runPruneClusters(dryRun, force bool) error {
	return app.withExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host) error {
		if err := authz.RequireOwner(h, "prune clusters"); err != nil {
			return err
		}
		opts := prune.Options{Clusters: true, Force: force}
		plan, err := prune.BuildPlan(ctx, exec, opts)
		if err != nil {
			return err
		}
		if dryRun {
			if app.Out.JSON {
				return app.Out.PrintJSON(plan)
			}
			for _, c := range plan.Candidates {
				app.Out.Info("[%s] %s %s — %s", c.Kind, c.ID, c.Name, c.Reason)
			}
			return nil
		}
		count, _ := share.ApprovedCount(ctx, exec)
		if err := authz.ConfirmDestructive(count, "prune clusters", app.ForceYes); err != nil {
			return err
		}
		if !app.ForceYes && !force {
			return fmt.Errorf("aborted — re-run with --yes to confirm cluster prune")
		}
		result, err := prune.Execute(ctx, exec, plan, opts)
		if err != nil {
			return err
		}
		if app.Out.JSON {
			return app.Out.PrintJSON(result)
		}
		app.Out.Success("Pruned %d cluster(s)", len(result.Removed))
		return nil
	})
}

func (app *App) runPruneMachines(dryRun, force bool) error {
	return app.withExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host) error {
		if err := authz.RequireOwner(h, "prune machines"); err != nil {
			return err
		}
		opts := prune.Options{Machines: true, Force: force}
		plan, err := prune.BuildPlan(ctx, exec, opts)
		if err != nil {
			return err
		}
		if dryRun {
			if app.Out.JSON {
				return app.Out.PrintJSON(plan)
			}
			for _, c := range plan.Candidates {
				app.Out.Info("[%s] %s %s — %s", c.Kind, c.ID, c.Name, c.Reason)
			}
			return nil
		}
		count, _ := share.ApprovedCount(ctx, exec)
		if err := authz.ConfirmDestructive(count, "prune machines", app.ForceYes); err != nil {
			return err
		}
		if !app.ForceYes && !force {
			return fmt.Errorf("aborted — re-run with --yes to confirm machine prune")
		}
		result, err := prune.Execute(ctx, exec, plan, opts)
		if err != nil {
			return err
		}
		if app.Out.JSON {
			return app.Out.PrintJSON(result)
		}
		app.Out.Success("Pruned %d machine(s)", len(result.Removed))
		return nil
	})
}

func (app *App) runPrune(dryRun, volumes, force bool) error {
	return app.withExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host) error {
		opts := prune.Options{Volumes: volumes, Force: force}
		plan, err := prune.BuildPlan(ctx, exec, opts)
		if err != nil {
			return err
		}
		if dryRun {
			if app.Out.JSON {
				return app.Out.PrintJSON(plan)
			}
			for _, c := range plan.Candidates {
				app.Out.Info("[%s] %s %s — %s", c.Kind, c.ID, c.Name, c.Reason)
			}
			app.Out.Info("Protected: %s", plan.ProtectedNote)
			return nil
		}
		count, _ := share.ApprovedCount(ctx, exec)
		label := "prune"
		if volumes {
			label = "prune volumes"
		}
		if err := authz.ConfirmDestructive(count, label, app.ForceYes); err != nil {
			return err
		}
		if volumes && !app.ForceYes {
			var names []string
			for _, c := range plan.Candidates {
				if c.Kind == "volume" {
					names = append(names, c.Name)
				}
			}
			if len(names) > 0 {
				app.Out.Info("Volumes to remove: %s", strings.Join(names, ", "))
			}
			return fmt.Errorf("aborted — re-run with --yes to confirm volume prune")
		}
		if volumes && !force {
			var names []string
			for _, c := range plan.Candidates {
				if c.Kind == "volume" {
					names = append(names, c.Name)
				}
			}
			if len(names) > 0 {
				app.Out.Info("Volumes to remove: %s", strings.Join(names, ", "))
			}
		}
		result, err := prune.Execute(ctx, exec, plan, opts)
		if err != nil {
			return err
		}
		if app.Out.JSON {
			return app.Out.PrintJSON(result)
		}
		app.Out.Success("Pruned %d resource(s)", len(result.Removed))
		if result.ReclaimedBytes > 0 {
			app.Out.Info("Reclaimed approximately %s", formatBytes(uint64(result.ReclaimedBytes)))
		}
		return nil
	})
}

func (app *App) resetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reset",
		Short: "Clear all local Outpost configuration",
		Long: "Delete ~/.outpost, including registered hosts, SSH identities, sessions, and kubeconfigs. " +
			"Remote servers are not affected. Repository .outpost/project.yaml files are not removed.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !app.ForceYes {
				if err := authz.ConfirmPrompt("This will delete all local Outpost configuration. Remote servers are not affected"); err != nil {
					return err
				}
			}
			if err := config.ResetLocal(); err != nil {
				return err
			}
			app.Out.Success("Local Outpost configuration cleared")
			return nil
		},
	}
}

func formatDuration(seconds uint64) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%dm", seconds/60)
	}
	if seconds < 86400 {
		return fmt.Sprintf("%dh", seconds/3600)
	}
	return fmt.Sprintf("%dd", seconds/86400)
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}
