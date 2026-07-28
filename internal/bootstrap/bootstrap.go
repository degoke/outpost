package bootstrap

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/degoke/outpost/internal/transport"
)

const bootstrapScript = `
set -e
OUTPOST_BASE="/var/lib/outpost"
need_sudo=""
if [ "$(id -u)" -ne 0 ]; then
  if command -v sudo >/dev/null 2>&1; then
    need_sudo="sudo"
  else
    echo "OUTPOST_ERROR: root or sudo required to install Docker"
    exit 1
  fi
fi

install_docker_debian() {
  if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
    return 0
  fi
  $need_sudo apt-get update -qq
  $need_sudo apt-get install -y -qq ca-certificates curl gnupg
  $need_sudo install -m 0755 -d /etc/apt/keyrings
  curl -fsSL https://download.docker.com/linux/$(. /etc/os-release && echo "$ID")/gpg | $need_sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg 2>/dev/null || true
  echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/$(. /etc/os-release && echo "$ID") $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | $need_sudo tee /etc/apt/sources.list.d/docker.list >/dev/null
  $need_sudo apt-get update -qq
  $need_sudo apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
}

install_docker_rhel() {
  if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
    return 0
  fi
  $need_sudo yum install -y -q yum-utils
  $need_sudo yum-config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo
  $need_sudo yum install -y -q docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
  $need_sudo systemctl enable --now docker 2>/dev/null || $need_sudo service docker start 2>/dev/null || true
}

detect_family() {
  if [ -f /etc/os-release ]; then
    . /etc/os-release
    case "$ID" in
      ubuntu|debian) echo debian; return ;;
      amzn|rhel|centos|fedora|rocky|almalinux) echo rhel; return ;;
    esac
  fi
  echo unknown
}

family=$(detect_family)
case "$family" in
  debian) install_docker_debian ;;
  rhel) install_docker_rhel ;;
  *)
    echo "OUTPOST_ERROR: unsupported distribution — install Docker manually, then run 'outpost host verify' again"
    exit 1
    ;;
esac

if ! docker compose version >/dev/null 2>&1; then
  echo "OUTPOST_ERROR: Docker Compose plugin not available after install"
  exit 1
fi

$need_sudo mkdir -p "$OUTPOST_BASE/projects" "$OUTPOST_BASE/share"
current_user="${SUDO_USER:-$USER}"
if [ -n "$current_user" ] && [ "$current_user" != "root" ]; then
  $need_sudo chown -R "$current_user:$current_user" "$OUTPOST_BASE"
fi
$need_sudo chmod 755 "$OUTPOST_BASE"

if [ -n "$current_user" ] && [ "$current_user" != "root" ]; then
  if ! id -nG "$current_user" | grep -qw docker; then
    $need_sudo usermod -aG docker "$current_user" 2>/dev/null || true
    echo "OUTPOST_WARN: added $current_user to docker group — log out and back in if docker permission errors occur"
  fi
fi

# Ensure share authorized_keys file exists
$need_sudo touch "$OUTPOST_BASE/share/authorized_keys"
$need_sudo chmod 600 "$OUTPOST_BASE/share/authorized_keys"

# Merge authorized_keys include hint
auth_keys="$HOME/.ssh/authorized_keys"
if [ -f "$auth_keys" ] && ! grep -q 'outpost/share/authorized_keys' "$auth_keys" 2>/dev/null; then
  mkdir -p "$HOME/.ssh"
  echo "# Outpost shared access keys" >> "$auth_keys"
  echo "include /var/lib/outpost/share/authorized_keys" >> "$auth_keys" 2>/dev/null || true
fi

echo "OUTPOST_OK: bootstrap complete"
`

func Ensure(ctx context.Context, exec transport.Executor) error {
	if ok, err := checkDocker(ctx, exec); err != nil {
		return err
	} else if ok {
		return ensureDirs(ctx, exec)
	}
	var stderr strings.Builder
	code, err := exec.Run(ctx, bootstrapScript, transport.RunOpts{Stderr: &stderr})
	out := stderr.String()
	if err != nil {
		return err
	}
	if code != 0 {
		if strings.Contains(out, "OUTPOST_ERROR:") {
			return fmt.Errorf("%s", strings.TrimSpace(strings.Split(out, "OUTPOST_ERROR:")[1]))
		}
		return fmt.Errorf("bootstrap failed (exit %d): %s", code, strings.TrimSpace(out))
	}
	if strings.Contains(out, "OUTPOST_WARN:") {
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, "OUTPOST_WARN:") {
				msg := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "OUTPOST_WARN:"))
				fmt.Fprintf(os.Stderr, "warning: %s\n", msg)
			}
		}
	}
	return ensureDirs(ctx, exec)
}

func checkDocker(ctx context.Context, exec transport.Executor) (bool, error) {
	code, err := exec.Run(ctx, "command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1", transport.RunOpts{})
	if err != nil {
		return false, err
	}
	return code == 0, nil
}

// EnsureInspectTools verifies basic host inspection utilities are available.
func EnsureInspectTools(ctx context.Context, exec transport.Executor) error {
	cmd := `command -v free >/dev/null && command -v df >/dev/null && command -v du >/dev/null`
	code, err := exec.Run(ctx, cmd, transport.RunOpts{})
	if err != nil {
		return err
	}
	if code != 0 {
		install := `
need_sudo=""
if [ "$(id -u)" -ne 0 ]; then need_sudo="sudo"; fi
if command -v apt-get >/dev/null 2>&1; then
  $need_sudo apt-get update -qq && $need_sudo apt-get install -y -qq procps coreutils
elif command -v yum >/dev/null 2>&1; then
  $need_sudo yum install -y -q procps-ng coreutils
fi
`
		_, _ = exec.Run(ctx, install, transport.RunOpts{})
	}
	return nil
}

const kubernetesToolsScript = `
set -e
if command -v kubectl >/dev/null 2>&1 && command -v kind >/dev/null 2>&1; then
  exit 0
fi
need_sudo=""
if [ "$(id -u)" -ne 0 ]; then need_sudo="sudo"; fi
if ! command -v kubectl >/dev/null 2>&1; then
  curl -fsSL "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl" -o /tmp/kubectl
  chmod +x /tmp/kubectl
  $need_sudo mv /tmp/kubectl /usr/local/bin/kubectl
fi
if ! command -v kind >/dev/null 2>&1; then
  curl -fsSL https://kind.sigs.k8s.io/dl/v0.24.0/kind-linux-amd64 -o /tmp/kind
  chmod +x /tmp/kind
  $need_sudo mv /tmp/kind /usr/local/bin/kind
fi
`

func EnsureKubernetesTools(ctx context.Context, exec transport.Executor) error {
	code, err := exec.Run(ctx, "command -v kind >/dev/null 2>&1 && command -v kubectl >/dev/null 2>&1", transport.RunOpts{})
	if err != nil {
		return err
	}
	if code == 0 {
		return nil
	}
	var stderr strings.Builder
	code, err = exec.Run(ctx, kubernetesToolsScript, transport.RunOpts{Stderr: &stderr})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("kubernetes tools install failed: %s", strings.TrimSpace(stderr.String()))
	}
	return nil
}

func ensureDirs(ctx context.Context, exec transport.Executor) error {
	cmd := `mkdir -p /var/lib/outpost/projects /var/lib/outpost/share /var/lib/outpost/clusters /var/lib/outpost/machines && (chown -R "$USER:$USER" /var/lib/outpost 2>/dev/null || sudo chown -R "$USER:$USER" /var/lib/outpost) && test -d /var/lib/outpost/projects`
	code, err := exec.Run(ctx, cmd, transport.RunOpts{})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("could not create /var/lib/outpost directories — ensure you have write permissions or run bootstrap with sudo")
	}
	_ = EnsureInspectTools(ctx, exec)
	return nil
}

const incusToolsScript = `
set -e
need_sudo=""
if [ "$(id -u)" -ne 0 ]; then need_sudo="sudo"; fi

detect_family() {
  if [ -f /etc/os-release ]; then
    . /etc/os-release
    case "$ID" in
      ubuntu|debian) echo debian; return ;;
    esac
  fi
  echo unknown
}

if ! command -v incus >/dev/null 2>&1; then
  family=$(detect_family)
  case "$family" in
    debian)
      $need_sudo apt-get update -qq
      $need_sudo apt-get install -y -qq incus
      ;;
    *)
      echo "OUTPOST_ERROR: unsupported distribution for Incus — install incus manually, then run again"
      exit 1
      ;;
  esac
fi

if ! incus list >/dev/null 2>&1; then
  $need_sudo incus admin init --auto
fi

current_user="${SUDO_USER:-$USER}"
if [ -n "$current_user" ] && [ "$current_user" != "root" ]; then
  if ! id -nG "$current_user" | grep -qw incus-admin; then
    $need_sudo usermod -aG incus-admin "$current_user" 2>/dev/null || true
    echo "OUTPOST_WARN: added $current_user to incus-admin group — log out and back in if incus permission errors occur"
  fi
fi
`

func EnsureIncus(ctx context.Context, exec transport.Executor) error {
	code, err := exec.Run(ctx, "command -v incus >/dev/null 2>&1 && incus list >/dev/null 2>&1", transport.RunOpts{})
	if err != nil {
		return err
	}
	if code == 0 {
		return nil
	}
	var stderr strings.Builder
	code, err = exec.Run(ctx, incusToolsScript, transport.RunOpts{Stderr: &stderr})
	out := stderr.String()
	if err != nil {
		return err
	}
	if code != 0 {
		if strings.Contains(out, "OUTPOST_ERROR:") {
			return fmt.Errorf("%s", strings.TrimSpace(strings.Split(out, "OUTPOST_ERROR:")[1]))
		}
		return fmt.Errorf("incus install failed: %s", strings.TrimSpace(out))
	}
	if strings.Contains(out, "OUTPOST_WARN:") {
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, "OUTPOST_WARN:") {
				msg := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "OUTPOST_WARN:"))
				fmt.Fprintf(os.Stderr, "warning: %s\n", msg)
			}
		}
	}
	return nil
}
