package aws

import (
	"encoding/base64"
	"fmt"
)

const outpostUser = "outpost"

func cloudInitUserData(sshPublicKey string) string {
	// Keep cloud-init minimal so SSH is available quickly; bootstrap.Ensure installs Docker over SSH.
	encodedKey := base64.StdEncoding.EncodeToString([]byte(sshPublicKey))
	script := fmt.Sprintf(`#!/bin/bash
set -euo pipefail

if ! id -u %s >/dev/null 2>&1; then
  useradd -m -s /bin/bash -G sudo %s
fi
mkdir -p /home/%s/.ssh
echo '%s' | base64 --decode >> /home/%s/.ssh/authorized_keys
chmod 700 /home/%s/.ssh
chmod 600 /home/%s/.ssh/authorized_keys
chown -R %s:%s /home/%s/.ssh
echo '%s ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/outpost
chmod 440 /etc/sudoers.d/outpost

OUTPOST_BASE="/var/lib/outpost"
mkdir -p "$OUTPOST_BASE/projects" "$OUTPOST_BASE/share" "$OUTPOST_BASE/clusters" "$OUTPOST_BASE/machines"
chown -R outpost:outpost "$OUTPOST_BASE"
chmod 755 "$OUTPOST_BASE"
touch "$OUTPOST_BASE/share/authorized_keys"
chmod 600 "$OUTPOST_BASE/share/authorized_keys"

echo "OUTPOST_CLOUD_INIT_OK"
`, outpostUser, outpostUser, outpostUser, encodedKey, outpostUser, outpostUser, outpostUser, outpostUser, outpostUser, outpostUser, outpostUser)
	return base64.StdEncoding.EncodeToString([]byte(script))
}

// CloudInitUserDataForTest exposes user-data generation for tests.
func CloudInitUserDataForTest(sshPublicKey string) string {
	return cloudInitUserData(sshPublicKey)
}
