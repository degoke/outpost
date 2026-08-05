#!/usr/bin/env bash
# Prepare the current machine as a loopback Outpost SSH host for e2e tests.
# Prints OUTPOST_E2E_SSH_KEY=<path> on stdout for the harness to parse.
set -euo pipefail

STATE_DIR="${OUTPOST_E2E_STATE_DIR:-/tmp/outpost-e2e}"
mkdir -p "$STATE_DIR"
KEY="$STATE_DIR/loopback_key"

ssh_listening() {
  if command -v nc >/dev/null 2>&1; then
    nc -z 127.0.0.1 22 >/dev/null 2>&1
    return
  fi
  bash -c 'echo >/dev/tcp/127.0.0.1/22' >/dev/null 2>&1
}

ensure_macos_remote_login() {
  [[ "$(uname -s)" == Darwin ]] || return 0
  if ssh_listening; then
    return 0
  fi
  if command -v systemsetup >/dev/null 2>&1; then
    echo "Enabling macOS Remote Login for loopback e2e..." >&2
    sudo systemsetup -setremotelogin on >/dev/null 2>&1 || true
    for _ in $(seq 1 10); do
      if ssh_listening; then
        return 0
      fi
      sleep 1
    done
  fi
  cat >&2 <<'EOF'
macOS loopback e2e requires SSH on 127.0.0.1:22.
Enable Remote Login in System Settings > General > Sharing,
or run: sudo systemsetup -setremotelogin on
EOF
  exit 1
}

ensure_macos_remote_login

if ! command -v sshd >/dev/null 2>&1; then
  if command -v apt-get >/dev/null 2>&1; then
    sudo apt-get update -qq
    sudo apt-get install -y -qq openssh-server
  elif [[ "$(uname -s)" != Darwin ]]; then
    echo "openssh-server is required for loopback e2e" >&2
    exit 1
  fi
fi

if ! ssh_listening; then
  if [[ "$(uname -s)" == Darwin ]]; then
    ensure_macos_remote_login
  else
    sudo systemctl start ssh 2>/dev/null || sudo service ssh start 2>/dev/null || true
  fi
fi

if ! ssh_listening; then
  echo "loopback e2e could not reach SSH on 127.0.0.1:22" >&2
  exit 1
fi

if [ ! -f "$KEY" ]; then
  ssh-keygen -t ed25519 -f "$KEY" -N "" -q
fi

mkdir -p "${HOME}/.ssh"
chmod 700 "${HOME}/.ssh"
touch "${HOME}/.ssh/authorized_keys"
chmod 600 "${HOME}/.ssh/authorized_keys"
if ! grep -qF "$(cat "${KEY}.pub")" "${HOME}/.ssh/authorized_keys" 2>/dev/null; then
  cat "${KEY}.pub" >> "${HOME}/.ssh/authorized_keys"
fi

if getent group docker >/dev/null 2>&1; then
  sudo usermod -aG docker "${USER}" 2>/dev/null || true
fi

echo "OUTPOST_E2E_SSH_KEY=$KEY"
