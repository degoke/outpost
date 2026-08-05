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
	  curl -fsSL https://download.docker.com/linux/$(. /etc/os-release && echo "$ID")/gpg | $need_sudo gpg --dearmor --yes -o /etc/apt/keyrings/docker.gpg
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

expand_root_filesystem() {
  marker="$OUTPOST_BASE/.rootfs-expanded"
  if [ -f "$marker" ]; then return 0; fi
  root_source=$(findmnt -n -o SOURCE / 2>/dev/null || true)
  root_source=$(readlink -f "$root_source" 2>/dev/null || true)
  root_fstype=$(findmnt -n -o FSTYPE / 2>/dev/null || true)
  if [ -z "$root_source" ] || [ "$root_source" = "/" ]; then return 0; fi
  if ! command -v growpart >/dev/null 2>&1; then
    if [ "$family" = "debian" ]; then
      $need_sudo apt-get update -qq
      $need_sudo apt-get install -y -qq cloud-guest-utils
    elif [ "$family" = "rhel" ]; then
      $need_sudo yum install -y -q cloud-utils-growpart
    fi
  fi
  if command -v growpart >/dev/null 2>&1; then
    case "$root_source" in
      /dev/nvme[0-9]n[0-9]p[0-9]*)
        root_disk=$(printf '%s' "$root_source" | sed -E 's/p[0-9]+$//')
        root_part=$(printf '%s' "$root_source" | sed -E 's/^.*p([0-9]+)$/\1/')
        ;;
      /dev/xvd[a-z][0-9]*|/dev/sd[a-z][0-9]*)
        root_disk=$(printf '%s' "$root_source" | sed -E 's/[0-9]+$//')
        root_part=$(printf '%s' "$root_source" | sed -E 's/^.*[^0-9]([0-9]+)$/\1/')
        ;;
      *) root_disk=""; root_part="" ;;
    esac
    if [ -n "$root_disk" ] && [ -n "$root_part" ]; then
      $need_sudo growpart "$root_disk" "$root_part" >/dev/null 2>&1 || true
    fi
  fi
  case "$root_fstype" in
    ext2|ext3|ext4) $need_sudo resize2fs "$root_source" >/dev/null 2>&1 || true ;;
    xfs) $need_sudo xfs_growfs / >/dev/null 2>&1 || true ;;
    btrfs) $need_sudo btrfs filesystem resize max / >/dev/null 2>&1 || true ;;
  esac
  $need_sudo touch "$marker"
}

expand_root_filesystem

$need_sudo mkdir -p "$OUTPOST_BASE/projects" "$OUTPOST_BASE/share"
current_user="${SUDO_USER:-$USER}"
if [ -n "$current_user" ] && [ "$current_user" != "root" ]; then
  $need_sudo chown -R "$current_user:$current_user" "$OUTPOST_BASE"
fi
$need_sudo chmod 755 "$OUTPOST_BASE"

# Install the forced command used by shared-member SSH keys. Members must not
# be able to bypass the Outpost command policy by invoking ssh directly.
member_shell_tmp=$(mktemp)
cat > "$member_shell_tmp" <<'OUTPOST_MEMBER_SHELL'
#!/bin/sh
set -eu

cmd=${SSH_ORIGINAL_COMMAND:-}
deny() {
  echo "OUTPOST_ERROR: shared member SSH keys may only run read-only Outpost operations" >&2
  exit 126
}
[ -n "$cmd" ] || deny

# Outpost's project executor prefixes commands with a safe project directory.
# Strip only that exact shape; the path is restricted to the project root.
case "$cmd" in
  cd\ /var/lib/outpost/projects/*\ \&\&\ *)
    project_path=${cmd#cd /var/lib/outpost/projects/}
    project_path=${project_path%% && *}
    cmd=${cmd#* && }
    case "$project_path" in
      ""|*[!A-Za-z0-9._/-]*|*..*) deny ;;
    esac
    ;;
esac

# These are fixed inspection commands emitted by the CLI. Keep the shell
# operators only for these literal commands; all user-controlled commands are
# checked below before being passed to a shell.
case "$cmd" in
  "echo outpost-ok"|"echo OUTPOST_SSH_OK"|\
  "nproc"|"free -b | head -2"|"df -B1 / | tail -1"|"cat /proc/uptime"|"head -1 /proc/stat"|\
  "docker info >/dev/null 2>&1"|"docker images -q | wc -l"|"docker volume ls -q | wc -l"|\
  "docker network ls --filter dangling=true -q | wc -l"|"docker system df"|"docker compose ls --format json"|\
  "cat '/var/lib/outpost/share/manifest.yaml'"|\
  "incus list --format json 2>/dev/null || true"|"sudo incus list >/dev/null 2>&1"|\
  "kind get clusters 2>/dev/null || true"|"k3d cluster list 2>/dev/null | awk 'NR>1 && NF {print \$1}' || true"|\
  "ls -1 /var/lib/outpost/machines 2>/dev/null || true"|"ls -1 /var/lib/outpost/clusters 2>/dev/null || true"|\
  "find /var/lib/outpost/projects -path '*/.upload-tmp/*' -type f 2>/dev/null || true"|\
  "du -sb /var/lib/outpost/projects /var/lib/outpost/share /var/lib/outpost/machines /var/lib/outpost/toolchains /var/lib/outpost/clusters 2>/dev/null || true")
    exec /bin/sh -c "$cmd"
    ;;
esac

case "$cmd" in
  "cat '/var/lib/outpost/machines/"*"/meta.yaml'"|"cat '/var/lib/outpost/clusters/"*"/meta.yaml'")
    case "$cmd" in *".."*|*" "*) deny ;; esac
    exec /bin/sh -c "$cmd"
    ;;
esac

# No shell control operators, redirections, command substitution, or escaped
# characters are accepted in user-controlled read-only commands.
backtick=$(printf '\140')
newline=$(printf '\n')
carriage_return=$(printf '\r')
case "$cmd" in
  *";"*|*"&"*|*"|"*|*"<"*|*">"*|*'$('*|*"$backtick"*|*"$newline"*|*"$carriage_return"*|*"\\"*) deny ;;
esac
case "$cmd" in
  "docker ps"|"docker ps "*|"docker logs"|"docker logs "*|\
  "docker stats"|"docker stats "*|"docker top"|"docker top "*|\
  "docker version"|"docker version "*|"docker info"|"docker info "*)
    exec /bin/sh -c "$cmd"
    ;;
  "docker compose "*)
    # Compose options may precede only the read-only ps/logs subcommands.
    # A different Compose subcommand therefore cannot pass this parser.
    normalized=$(printf '%s' "$cmd" | tr -d "'\"")
    set -- $normalized
    [ "${1:-}" = "docker" ] && [ "${2:-}" = "compose" ] || deny
    shift 2
    subcommand=""
    while [ "$#" -gt 0 ]; do
      case "$1" in
        -p|-f|--project-name|--file)
          [ "$#" -ge 2 ] || deny
          shift 2
          ;;
        -*) shift ;;
        ps|logs) subcommand=$1; shift; break ;;
        *) deny ;;
      esac
    done
    [ "$subcommand" = "ps" ] || [ "$subcommand" = "logs" ] || deny
    exec /bin/sh -c "$cmd"
    ;;
  "incus list"|"incus list "*|"sudo incus list"|"sudo incus list "*)
    exec /bin/sh -c "$cmd"
    ;;
  "kind get clusters"*|"k3d cluster list"*)
    exec /bin/sh -c "$cmd"
    ;;
esac

deny
OUTPOST_MEMBER_SHELL
$need_sudo install -m 0755 "$member_shell_tmp" /usr/local/bin/outpost-member-shell
rm -f "$member_shell_tmp"

if [ -n "$current_user" ] && [ "$current_user" != "root" ]; then
  if ! id -nG "$current_user" | grep -qw docker; then
    $need_sudo usermod -aG docker "$current_user" 2>/dev/null || true
    echo "OUTPOST_WARN: added $current_user to docker group — log out and back in if docker permission errors occur"
  fi
fi

# Ensure share authorized_keys file exists and is readable by the outpost user.
# sshd opens included paths as the authenticated user, not root.
$need_sudo install -o outpost -g outpost -m 600 /dev/null "$OUTPOST_BASE/share/authorized_keys"

# Keep the share manifest present even before the first invitation is created.
# Read-only host checks issue this path directly and should not emit a missing
# file error on a freshly bootstrapped host.
share_manifest="$OUTPOST_BASE/share/manifest.yaml"
if [ ! -f "$share_manifest" ]; then
  cat > "$share_manifest" <<'OUTPOST_MANIFEST'
version: 1
invitations: []
devices: []
OUTPOST_MANIFEST
  $need_sudo chown outpost:outpost "$share_manifest"
  $need_sudo chmod 600 "$share_manifest"
fi

# Merge authorized_keys include hint. Keep the include path readable by the
# outpost user (see install above) so sshd can open it during authentication.
auth_keys="$HOME/.ssh/authorized_keys"
if [ -f "$auth_keys" ] && ! grep -q 'outpost/share/authorized_keys' "$auth_keys" 2>/dev/null; then
  mkdir -p "$HOME/.ssh"
  tmp_keys=$(mktemp)
  {
    echo "# Outpost shared access keys"
    echo "include $OUTPOST_BASE/share/authorized_keys"
    cat "$auth_keys"
  } > "$tmp_keys"
  mv "$tmp_keys" "$auth_keys"
  chmod 600 "$auth_keys"
fi

echo "OUTPOST_OK: bootstrap complete"
`

func Ensure(ctx context.Context, exec transport.Executor) error {
	if ok, err := checkDocker(ctx, exec); err != nil {
		return err
	} else if ok {
		code, err := exec.Run(ctx, bootstrapScript, transport.RunOpts{})
		if err != nil {
			return err
		}
		if code != 0 {
			return fmt.Errorf("remote bootstrap failed (exit %d)", code)
		}
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
need_sudo=""
if [ "$(id -u)" -ne 0 ]; then need_sudo="sudo"; fi
if [ -z "$need_sudo" ] && [ "$(id -u)" -ne 0 ]; then
  echo "OUTPOST_ERROR: root or sudo is required to install Kubernetes tools"
  exit 1
fi

install_packages() {
  packages="$1"
  if command -v apt-get >/dev/null 2>&1; then
    $need_sudo apt-get update -qq
    $need_sudo apt-get install -y -qq $packages
  elif command -v yum >/dev/null 2>&1; then
    $need_sudo yum install -y -q $packages
  else
    echo "OUTPOST_ERROR: unsupported project-container distribution — install kubectl, kind/k3d, curl, and Docker CLI manually"
    exit 1
  fi
}

if ! command -v curl >/dev/null 2>&1; then
  install_packages "ca-certificates curl"
fi
if ! command -v docker >/dev/null 2>&1; then
  if command -v apt-get >/dev/null 2>&1; then
    install_packages "docker.io"
  else
    install_packages "docker"
  fi
fi
case "$(uname -m)" in
  x86_64|amd64) arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *) echo "OUTPOST_ERROR: unsupported project-container architecture $(uname -m)"; exit 1 ;;
esac
if ! command -v kubectl >/dev/null 2>&1; then
	version="$(curl -fsSL https://dl.k8s.io/release/stable.txt)"
	url="https://dl.k8s.io/release/$version/bin/linux/$arch/kubectl"
	curl -fsSL "$url" -o /tmp/kubectl
	curl -fsSL "$url.sha256" -o /tmp/kubectl.sha256
	printf '%s  %s\n' "$(cat /tmp/kubectl.sha256)" /tmp/kubectl | sha256sum -c -
	  chmod +x /tmp/kubectl
	  $need_sudo mv /tmp/kubectl /usr/local/bin/kubectl
fi
if ! command -v kind >/dev/null 2>&1; then
	kind_url="https://kind.sigs.k8s.io/dl/v0.24.0/kind-linux-$arch"
	curl -fsSL "$kind_url" -o /tmp/kind
	curl -fsSL "$kind_url.sha256sum" -o /tmp/kind.sha256sum
	printf '%s  %s\n' "$(awk '{print $1}' /tmp/kind.sha256sum)" /tmp/kind | sha256sum -c -
	  chmod +x /tmp/kind
	  $need_sudo mv /tmp/kind /usr/local/bin/kind
fi
if ! command -v k3d >/dev/null 2>&1; then
	  k3d_url="https://github.com/k3d-io/k3d/releases/download/v5.8.3/k3d-linux-$arch"
	  curl -fsSL "$k3d_url" -o /tmp/k3d
	  curl -fsSL "https://github.com/k3d-io/k3d/releases/download/v5.8.3/checksums.txt" -o /tmp/k3d-checksums.txt
	  k3d_sum="$(grep -F "k3d-linux-$arch" /tmp/k3d-checksums.txt | awk '{print $1; exit}')"
	  [ -n "$k3d_sum" ] || { echo "OUTPOST_ERROR: no k3d checksum found"; exit 1; }
	  printf '%s  %s\n' "$k3d_sum" /tmp/k3d | sha256sum -c -
	  chmod +x /tmp/k3d
	  $need_sudo mv /tmp/k3d /usr/local/bin/k3d
fi
`

func EnsureKubernetesTools(ctx context.Context, exec transport.Executor) error {
	code, err := exec.Run(ctx, "command -v kind >/dev/null 2>&1 && command -v kubectl >/dev/null 2>&1 && command -v k3d >/dev/null 2>&1", transport.RunOpts{})
	if err != nil {
		return err
	}
	if code == 0 {
		return nil
	}
	var stderr strings.Builder
	var stdout strings.Builder
	code, err = exec.Run(ctx, kubernetesToolsScript, transport.RunOpts{Stdout: &stdout, Stderr: &stderr})
	if err != nil {
		return err
	}
	if code != 0 {
		message := strings.TrimSpace(strings.Join([]string{stdout.String(), stderr.String()}, "\n"))
		if marker := strings.Index(message, "OUTPOST_ERROR:"); marker >= 0 {
			message = strings.TrimSpace(message[marker+len("OUTPOST_ERROR:"):])
		}
		return fmt.Errorf("kubernetes tools install failed: %s", message)
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
	code, err := exec.Run(ctx, "command -v incus >/dev/null 2>&1 && (incus list >/dev/null 2>&1 || sudo incus list >/dev/null 2>&1)", transport.RunOpts{})
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
