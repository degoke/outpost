package environment

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/degoke/outpost/internal/cleanup"
	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/transport"
)

const (
	defaultImage   = "ubuntu:24.04"
	defaultShell   = "/bin/bash"
	defaultWorkdir = "/workspace"
)

// Manager owns the lifecycle of one project's remote development container.
type Manager struct {
	Exec          transport.Executor
	Project       *config.Project
	Cwd           string
	initErr       error
	autoToolchain toolchainSet
}

func New(exec transport.Executor, project *config.Project, cwd string) *Manager {
	m := &Manager{Exec: exec, Project: project, Cwd: cwd}
	m.initErr = applyDevcontainer(cwd, project)
	if m.initErr == nil && project != nil && project.Environment != nil &&
		strings.TrimSpace(project.Environment.Image) == "" &&
		strings.TrimSpace(project.Environment.BuildDockerfile) == "" {
		m.autoToolchain = detectToolchains(cwd)
	}
	return m
}

type toolchainSet struct {
	Go     string
	Node   bool
	PNPM   string
	Python bool
}

var goVersionPattern = regexp.MustCompile(`(?m)^\s*go\s+(\d+\.\d+(?:\.\d+)?)\s*$`)

func detectToolchains(cwd string) toolchainSet {
	var found toolchainSet
	_ = filepath.Walk(cwd, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if path != cwd && (name == ".git" || name == "node_modules" || name == ".venv" || name == "dist" || name == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		switch info.Name() {
		case "go.mod":
			data, readErr := os.ReadFile(path)
			if readErr == nil {
				match := goVersionPattern.FindSubmatch(data)
				if len(match) == 2 {
					found.Go = string(match[1])
				}
			}
		case "package.json":
			data, readErr := os.ReadFile(path)
			if readErr == nil {
				var pkg struct {
					PackageManager string `json:"packageManager"`
				}
				if json.Unmarshal(data, &pkg) == nil {
					found.Node = true
					if strings.HasPrefix(pkg.PackageManager, "pnpm@") {
						found.PNPM = strings.TrimPrefix(pkg.PackageManager, "pnpm@")
					}
				}
			}
		case "requirements.txt", "pyproject.toml":
			found.Python = true
		}
		return nil
	})
	return found
}

type devcontainerConfig struct {
	Image           string            `json:"image"`
	WorkspaceFolder string            `json:"workspaceFolder"`
	ForwardPorts    []json.RawMessage `json:"forwardPorts"`
	ContainerEnv    map[string]string `json:"containerEnv"`
	Mounts          []string          `json:"mounts"`
	Build           struct {
		Dockerfile string `json:"dockerfile"`
		Context    string `json:"context"`
	} `json:"build"`
}

func applyDevcontainer(cwd string, project *config.Project) error {
	if project == nil || project.Environment == nil || cwd == "" {
		return nil
	}
	path := filepath.Join(cwd, ".devcontainer", "devcontainer.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var dc devcontainerConfig
	if err := json.Unmarshal(stripJSONComments(data), &dc); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	if project.Environment.Image == "" {
		project.Environment.Image = dc.Image
	}
	if project.Environment.Workdir == "" {
		project.Environment.Workdir = dc.WorkspaceFolder
	}
	if len(project.Environment.Ports) == 0 {
		project.Environment.Ports = parsePorts(dc.ForwardPorts)
	}
	if len(project.Environment.Environment) == 0 {
		project.Environment.Environment = dc.ContainerEnv
	}
	if len(project.Environment.Volumes) == 0 {
		for _, mount := range dc.Mounts {
			parts := strings.Split(mount, ",")
			var source, target, kind string
			for _, part := range parts {
				kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
				if len(kv) != 2 {
					continue
				}
				switch kv[0] {
				case "source":
					source = kv[1]
				case "target":
					target = kv[1]
				case "type":
					kind = kv[1]
				}
			}
			if kind == "volume" && source != "" && target != "" {
				project.Environment.Volumes = append(project.Environment.Volumes, config.ProjectEnvMount{Name: source, Path: target})
			}
		}
	}
	project.Environment.DevcontainerFile = ".devcontainer/devcontainer.json"
	if project.Environment.BuildDockerfile == "" {
		project.Environment.BuildDockerfile = dc.Build.Dockerfile
	}
	if project.Environment.BuildContext == "" {
		project.Environment.BuildContext = dc.Build.Context
	}
	return nil
}

func parsePorts(values []json.RawMessage) []int {
	var ports []int
	for _, raw := range values {
		var number int
		if json.Unmarshal(raw, &number) == nil && number > 0 {
			ports = append(ports, number)
			continue
		}
		var text string
		if json.Unmarshal(raw, &text) != nil {
			continue
		}
		parts := strings.Split(text, ":")
		port, err := strconv.Atoi(parts[len(parts)-1])
		if err == nil && port > 0 {
			ports = append(ports, port)
		}
	}
	return ports
}

func stripJSONComments(data []byte) []byte {
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			lines[i] = ""
		}
	}
	return []byte(strings.Join(lines, "\n"))
}

func (m *Manager) Name() string {
	return "outpost-dev-" + config.SanitizeProjectName(m.Project.Name)
}

func (m *Manager) image() string {
	if m.Project.Environment != nil && strings.TrimSpace(m.Project.Environment.Image) != "" {
		return strings.TrimSpace(m.Project.Environment.Image)
	}
	if m.autoToolchain.Go != "" || m.autoToolchain.Node || m.autoToolchain.Python {
		return "outpost-dev-" + config.SanitizeProjectName(m.Project.Name) + ":auto"
	}
	return defaultImage
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (m *Manager) shell() string {
	if m.Project.Environment != nil && strings.TrimSpace(m.Project.Environment.Shell) != "" {
		return strings.TrimSpace(m.Project.Environment.Shell)
	}
	return defaultShell
}

func (m *Manager) workdir() string {
	if m.Project.Environment != nil && strings.TrimSpace(m.Project.Environment.Workdir) != "" {
		return strings.TrimSpace(m.Project.Environment.Workdir)
	}
	return defaultWorkdir
}

// Workdir returns the path where the remote project is mounted inside the
// managed development container.
func (m *Manager) Workdir() string {
	return m.workdir()
}

func (m *Manager) socketEnabled() bool {
	return m.Project.Environment == nil || m.Project.Environment.DockerSocket == nil || *m.Project.Environment.DockerSocket
}

func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func (m *Manager) exists(ctx context.Context) (bool, error) {
	code, err := m.Exec.Run(ctx, fmt.Sprintf("docker inspect %s >/dev/null 2>&1", quote(m.Name())), transport.RunOpts{})
	return code == 0, err
}

func (m *Manager) hasHostGateway(ctx context.Context) (bool, error) {
	var out strings.Builder
	code, err := m.Exec.Run(ctx, fmt.Sprintf("docker inspect --format '{{json .HostConfig.ExtraHosts}}' %s", quote(m.Name())), transport.RunOpts{Stdout: &out})
	if err != nil {
		return false, err
	}
	if code != 0 {
		return false, fmt.Errorf("could not inspect development container %q", m.Name())
	}
	return strings.Contains(out.String(), "host.docker.internal"), nil
}

// Ensure starts the project container and returns its name.
func (m *Manager) Ensure(ctx context.Context) error {
	if m.initErr != nil {
		return m.initErr
	}
	if err := transport.EnsureRemoteDir(m.Exec, m.Project.RemoteDir); err != nil {
		return err
	}
	cleanupOpts := cleanup.OptionsForProject(m.Project)
	cleanupOpts.IncludeDockerCache = false
	if err := cleanup.Project(ctx, m.Exec, m.Project, cleanupOpts); err != nil {
		return err
	}
	exists, err := m.exists(ctx)
	if err != nil {
		return err
	}
	if exists && m.Project.Kubernetes != nil {
		gateway, err := m.hasHostGateway(ctx)
		if err != nil {
			return err
		}
		if !gateway {
			// ExtraHosts cannot be added to an existing Docker container. The
			// project directory and dependency volumes are persistent, so
			// recreating this managed development container is safe and makes
			// upgrading an existing project deterministic.
			code, err := m.Exec.Run(ctx, fmt.Sprintf("docker rm -f %s", quote(m.Name())), transport.RunOpts{})
			if err != nil {
				return err
			}
			if code != 0 {
				return fmt.Errorf("could not recreate development container %q with Kubernetes host gateway", m.Name())
			}
			exists = false
		}
	}
	if exists {
		code, err := m.Exec.Run(ctx, fmt.Sprintf("docker start %s >/dev/null", quote(m.Name())), transport.RunOpts{})
		if err != nil {
			return err
		}
		if code != 0 {
			return fmt.Errorf("could not start development container %q", m.Name())
		}
		return m.ensureShellBootstrap(ctx)
	}
	if err := m.buildImage(ctx); err != nil {
		return err
	}

	args := []string{
		"docker run -d --name", quote(m.Name()), "--restart unless-stopped",
		"--label", quote("com.outpost.managed=true"),
		"--label", quote("com.outpost.project=" + m.Project.Name),
		"--workdir", quote(m.workdir()),
		"--volume", quote(m.Project.RemoteDir + ":" + m.workdir()),
		"--env", quote("TERM=xterm-256color"),
		"--env", quote("COLORTERM=truecolor"),
	}
	if m.Project.Kubernetes != nil {
		args = append(args, "--add-host", quote("host.docker.internal:host-gateway"))
	}
	if m.socketEnabled() {
		args = append(args, "--volume", quote("/var/run/docker.sock:/var/run/docker.sock"))
	}
	if m.Project.Environment != nil {
		for name, value := range m.Project.Environment.Environment {
			args = append(args, "--env", quote(name+"="+value))
		}
		for _, mount := range m.Project.Environment.Volumes {
			if strings.TrimSpace(mount.Name) == "" || strings.TrimSpace(mount.Path) == "" {
				continue
			}
			args = append(args, "--volume", quote(mount.Name+":"+mount.Path))
		}
	}
	for _, mount := range m.defaultDependencyVolumes() {
		args = append(args, "--volume", quote(mount))
	}
	args = append(args, quote(m.image()), "sleep", "infinity")
	cmd := strings.Join(args, " ")
	code, err := m.Exec.Run(ctx, cmd, transport.RunOpts{})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("could not create development container %q", m.Name())
	}
	return m.ensureShellBootstrap(ctx)
}

func (m *Manager) defaultDependencyVolumes() []string {
	if m.Project.Environment != nil && len(m.Project.Environment.Volumes) > 0 {
		return nil
	}
	prefix := "outpost-deps-" + config.SanitizeProjectName(m.Project.Name)
	var out []string
	if fileExists(filepath.Join(m.Cwd, "package.json")) {
		out = append(out, prefix+"-node:/workspace/node_modules")
	}
	if fileExists(filepath.Join(m.Cwd, "requirements.txt")) || fileExists(filepath.Join(m.Cwd, "pyproject.toml")) {
		out = append(out, prefix+"-python:/workspace/.venv")
	}
	if fileExists(filepath.Join(m.Cwd, "go.mod")) {
		out = append(out, prefix+"-go:/go")
	}
	return out
}

func (m *Manager) buildImage(ctx context.Context) error {
	if m.autoToolchain.Go != "" || m.autoToolchain.Node || m.autoToolchain.Python {
		return m.buildAutoToolchainImage(ctx)
	}
	if m.Project.Environment == nil || strings.TrimSpace(m.Project.Environment.BuildDockerfile) == "" {
		return nil
	}
	contextDir := strings.TrimSpace(m.Project.Environment.BuildContext)
	if contextDir == "" {
		contextDir = "."
	}
	dockerfile := m.Project.RemoteDir + "/.devcontainer/" + filepath.Base(m.Project.Environment.BuildDockerfile)
	tag := "outpost-dev-" + config.SanitizeProjectName(m.Project.Name) + ":dev"
	cmd := fmt.Sprintf("docker build -t %s -f %s %s", quote(tag), quote(dockerfile), quote(m.Project.RemoteDir+"/"+contextDir))
	code, err := m.Exec.Run(ctx, cmd, transport.RunOpts{})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("could not build development image %q", tag)
	}
	m.Project.Environment.Image = tag
	return nil
}

func (m *Manager) buildAutoToolchainImage(ctx context.Context) error {
	base := "ubuntu:24.04"
	var dockerfile strings.Builder
	dockerfile.WriteString("FROM " + base + "\n")
	dockerfile.WriteString("ENV DEBIAN_FRONTEND=noninteractive\n")
	dockerfile.WriteString("RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl git make ")
	if m.autoToolchain.Python {
		dockerfile.WriteString("python3 python3-pip python3-venv ")
	}
	dockerfile.WriteString("&& rm -rf /var/lib/apt/lists/*\n")
	dockerfile.WriteString(shellBootstrapDockerfile())
	if m.autoToolchain.Go != "" {
		goImage := "golang:" + m.autoToolchain.Go + "-bookworm"
		dockerfile.WriteString("COPY --from=" + goImage + " /usr/local/go /usr/local/go\n")
		dockerfile.WriteString("ENV PATH=/usr/local/go/bin:/go/bin:$PATH GOPATH=/go\n")
	}
	if m.autoToolchain.Node {
		dockerfile.WriteString("COPY --from=node:22-bookworm /usr/local/bin/node /usr/local/bin/node\n")
		dockerfile.WriteString("COPY --from=node:22-bookworm /usr/local/lib/node_modules /usr/local/lib/node_modules\n")
		dockerfile.WriteString("RUN ln -sf /usr/local/lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm && ln -sf /usr/local/lib/node_modules/corepack/dist/corepack.js /usr/local/bin/corepack\n")
		if m.autoToolchain.PNPM != "" {
			dockerfile.WriteString("RUN corepack enable && corepack prepare pnpm@" + m.autoToolchain.PNPM + " --activate\n")
		}
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(dockerfile.String()))
	tag := "outpost-dev-" + config.SanitizeProjectName(m.Project.Name) + ":auto"
	tmp := "/tmp/outpost-" + config.SanitizeProjectName(m.Project.Name) + "-Dockerfile"
	cmd := fmt.Sprintf("echo %s | base64 -d > %s && docker build -t %s -f %s %s; status=$?; rm -f %s; exit $status",
		quote(encoded), quote(tmp), quote(tag), quote(tmp), quote(m.Project.RemoteDir), quote(tmp))
	code, err := m.Exec.Run(ctx, cmd, transport.RunOpts{})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("could not build automatically detected development image %q", tag)
	}
	m.Project.Environment.Image = tag
	return nil
}

func (m *Manager) ExecCommand(ctx context.Context, command string, opts transport.RunOpts) (int, error) {
	if err := m.Ensure(ctx); err != nil {
		return 1, err
	}
	cmd := fmt.Sprintf("docker exec %s %s -lc %s", quote(m.Name()), quote(m.shell()), quote(command))
	return m.Exec.Run(ctx, cmd, opts)
}

func (m *Manager) Shell(ctx context.Context, opts transport.RunOpts) error {
	if err := m.Ensure(ctx); err != nil {
		return err
	}
	inner := fmt.Sprintf("cd %s && if [ -f .venv/bin/activate ]; then . .venv/bin/activate; fi; exec %s", quote(m.workdir()), m.shell())
	cmd := dockerExecInteractive(m.Name(), m.shell(), inner)
	return m.Exec.RunInteractive(ctx, cmd, opts)
}

func (m *Manager) ExecInteractiveCommand(ctx context.Context, command string, opts transport.RunOpts) error {
	if err := m.Ensure(ctx); err != nil {
		return err
	}
	cmd := dockerExecInteractive(m.Name(), m.shell(), command)
	return m.Exec.RunInteractive(ctx, cmd, opts)
}

// dockerExecInteractive runs a command in the managed container without
// allocating a second TTY. The SSH session already owns the terminal.
func dockerExecInteractive(name, shell, inner string) string {
	return fmt.Sprintf(
		"docker exec -i -e TERM=xterm-256color -e COLORTERM=truecolor %s %s -lc %s",
		quote(name), quote(shell), quote(inner),
	)
}

// shellBootstrapDockerfile returns Dockerfile lines that install starship and
// enable a colored interactive shell in auto-built development images.
func shellBootstrapDockerfile() string {
	return `RUN curl -fsSL https://starship.rs/install.sh | sh -s -- -y -b /usr/local/bin \
 && printf '%s\n' 'eval "$(starship init bash)"' 'export TERM=xterm-256color' 'export COLORTERM=truecolor' >> /etc/bash.bashrc
`
}

// ensureShellBootstrap installs starship and shell defaults in pre-built or
// custom development images that were not built with shellBootstrapDockerfile.
func (m *Manager) ensureShellBootstrap(ctx context.Context) error {
	cmd := fmt.Sprintf("docker exec %s bash -lc %s", quote(m.Name()), quote(shellBootstrapScript()))
	_, _ = m.Exec.Run(ctx, cmd, transport.RunOpts{})
	return nil
}

func shellBootstrapScript() string {
	return `if command -v starship >/dev/null 2>&1; then exit 0; fi
curl -fsSL https://starship.rs/install.sh | sh -s -- -y -b /usr/local/bin
grep -q 'starship init bash' /etc/bash.bashrc 2>/dev/null || printf '%s\n' 'eval "$(starship init bash)"' 'export TERM=xterm-256color' 'export COLORTERM=truecolor' >> /etc/bash.bashrc`
}

// ContainerExecutor exposes the managed project container as a transport
// executor. Commands run through docker exec; file transfers and network
// forwards continue to use the underlying remote host executor.
type ContainerExecutor struct {
	Host    transport.Executor
	Manager *Manager
}

func (e *ContainerExecutor) Run(ctx context.Context, cmd string, opts transport.RunOpts) (int, error) {
	return e.Manager.ExecCommand(ctx, cmd, opts)
}

func (e *ContainerExecutor) RunInteractive(ctx context.Context, cmd string, opts transport.RunOpts) error {
	return e.Manager.ExecInteractiveCommand(ctx, cmd, opts)
}

func (e *ContainerExecutor) Upload(local, remote string) error {
	return e.Host.Upload(local, remote)
}

func (e *ContainerExecutor) UploadBytes(data []byte, remote string) error {
	return e.Host.UploadBytes(data, remote)
}

func (e *ContainerExecutor) Download(remote string) ([]byte, error) {
	return e.Host.Download(remote)
}

func (e *ContainerExecutor) Forward(ctx context.Context, spec transport.ForwardSpec) (io.Closer, error) {
	return e.Host.Forward(ctx, spec)
}

func (e *ContainerExecutor) HostInfo() string {
	return e.Host.HostInfo()
}

func (m *Manager) Stop(ctx context.Context, remove bool) error {
	cmd := fmt.Sprintf("docker stop %s", quote(m.Name()))
	if remove {
		cmd += " && docker rm " + quote(m.Name())
	}
	code, err := m.Exec.Run(ctx, cmd, transport.RunOpts{})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("could not stop development container %q", m.Name())
	}
	return nil
}

func (m *Manager) Status(ctx context.Context) (string, error) {
	var out strings.Builder
	_, err := m.Exec.Run(ctx, fmt.Sprintf("docker inspect --format '{{.State.Status}}' %s 2>/dev/null || true", quote(m.Name())), transport.RunOpts{Stdout: &out})
	return strings.TrimSpace(out.String()), err
}
