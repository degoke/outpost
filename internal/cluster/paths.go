package cluster

import (
	"fmt"
	"path/filepath"

	"github.com/degoke/outpost/internal/config"
)

const remoteBase = config.DefaultRemoteBase + "/clusters"

func RemoteDir(name string) string {
	return filepath.Join(remoteBase, config.SanitizeClusterName(name))
}

func KindName(name string) string {
	return "outpost-" + config.SanitizeClusterName(name)
}

func RemoteKubeconfig(name string) string {
	return RemoteDir(name) + "/kubeconfig"
}

func LocalKubeconfigPath(hostName, clusterName string) (string, error) {
	dir, err := config.KubeconfigsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fmt.Sprintf("%s-%s.yaml", config.SanitizeClusterName(hostName), config.SanitizeClusterName(clusterName))), nil
}
