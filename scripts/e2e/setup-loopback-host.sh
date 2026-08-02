#!/usr/bin/env bash
# Prepare the current machine as a loopback Outpost SSH host for e2e tests.
# Prints OUTPOST_E2E_SSH_KEY=<path> on stdout for the harness to parse.
set -euo pipefail

STATE_DIR="${OUTPOST_E2E_STATE_DIR:-/tmp/outpost-e2e}"
mkdir -p "$STATE_DIR"
KEY="$STATE_DIR/loopback_key"

if ! command -v sshd >/dev/null 2>&1; then
  if command -v apt-get >/dev/null 2>&1; then
    sudo apt-get update -qq
    sudo apt-get install -y -qq openssh-server
  else
    echo "openssh-server is required for loopback e2e" >&2
    exit 1
  fi
fi

if ! pgrep -x sshd >/dev/null 2>&1 && ! pgrep -x ssh >/dev/null 2>&1; then
  sudo systemctl start ssh 2>/dev/null || sudo service ssh start 2>/dev/null || true
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
