package machine

import (
	"github.com/goke/outpost/internal/config"
)

const remoteBase = config.DefaultRemoteBase + "/machines"

func RemoteDir(name string) string {
	return remoteBase + "/" + config.SanitizeMachineName(name)
}

func IncusName(name string) string {
	return "outpost-" + config.SanitizeMachineName(name)
}
