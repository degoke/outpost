package machine

import (
	"strings"

	"github.com/degoke/outpost/internal/config"
)

const remoteBase = config.DefaultRemoteBase + "/machines"

func RemoteDir(name string) string {
	return remoteBase + "/" + config.SanitizeMachineName(name)
}

func IncusName(name string) string {
	return "outpost-" + config.SanitizeMachineName(name)
}

// incusInstancePath formats an instance file path for incus file push/pull.
// Use instance/path, not instance:/path — colons select a remote, not the instance name.
func incusInstancePath(incusName, remotePath string) string {
	remotePath = strings.TrimPrefix(strings.TrimSpace(remotePath), "/")
	return incusName + "/" + remotePath
}

// IncusInstancePathForTest exposes instance path formatting for tests.
func IncusInstancePathForTest(incusName, remotePath string) string {
	return incusInstancePath(incusName, remotePath)
}
