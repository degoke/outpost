package aws

import (
	"encoding/base64"
	"fmt"
)

const outpostUser = "outpost"

func cloudInitUserData(sshPublicKey string) string {
	script := fmt.Sprintf(`#!/bin/bash
set -euxo pipefail
export DEBIAN_FRONTEND=noninteractive

if ! id -u %s >/dev/null 2>&1; then
  useradd -m -s /bin/bash -G sudo,docker %s || useradd -m -s /bin/bash -G sudo %s
fi
mkdir -p /home/%s/.ssh
echo '%s' >> /home/%s/.ssh/authorized_keys
chmod 700 /home/%s/.ssh
chmod 600 /home/%s/.ssh/authorized_keys
chown -R %s:%s /home/%s/.ssh
echo '%s ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/outpost
chmod 440 /etc/sudoers.d/outpost

OUTPOST_BASE="/var/lib/outpost"
mkdir -p "$OUTPOST_BASE/projects" "$OUTPOST_BASE/share" "$OUTPOST_BASE/clusters"
chmod 755 "$OUTPOST_BASE"

install_docker_debian() {
  if command -v docker >/dev/null 2>&1; then return 0; fi
  apt-get update -qq
  apt-get install -y -qq ca-certificates curl gnupg procps coreutils
  install -m 0755 -d /etc/apt/keyrings
  curl -fsSL https://download.docker.com/linux/$(. /etc/os-release && echo "$ID")/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg 2>/dev/null || true
  echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/$(. /etc/os-release && echo "$ID") $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | tee /etc/apt/sources.list.d/docker.list >/dev/null
  apt-get update -qq
  apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
}

install_docker_debian
systemctl enable --now docker 2>/dev/null || service docker start 2>/dev/null || true
usermod -aG docker %s 2>/dev/null || true

touch "$OUTPOST_BASE/share/authorized_keys"
chmod 600 "$OUTPOST_BASE/share/authorized_keys"

# kind + kubectl (idempotent)
if ! command -v kubectl >/dev/null 2>&1; then
  curl -fsSL "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl" -o /usr/local/bin/kubectl
  chmod +x /usr/local/bin/kubectl
fi
if ! command -v kind >/dev/null 2>&1; then
  curl -fsSL https://kind.sigs.k8s.io/dl/v0.24.0/kind-linux-amd64 -o /usr/local/bin/kind
  chmod +x /usr/local/bin/kind
fi

echo "OUTPOST_CLOUD_INIT_OK"
`, outpostUser, outpostUser, outpostUser, outpostUser, sshPublicKey, outpostUser, outpostUser, outpostUser, outpostUser, outpostUser, outpostUser, outpostUser, outpostUser)
	return base64.StdEncoding.EncodeToString([]byte(script))
}

// CloudInitUserDataForTest exposes user-data generation for tests.
func CloudInitUserDataForTest(sshPublicKey string) string {
	return cloudInitUserData(sshPublicKey)
}
