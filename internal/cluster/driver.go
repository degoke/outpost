package cluster

import (
	"context"
	"fmt"
	"strings"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/inspect"
	"github.com/degoke/outpost/internal/transport"
	"gopkg.in/yaml.v3"
)

type Driver string

const (
	DriverKind Driver = "kind"
	DriverK3d  Driver = "k3d"
)

func ParseDriver(s string) (Driver, error) {
	switch Driver(strings.ToLower(strings.TrimSpace(s))) {
	case "", "kind":
		return DriverKind, nil
	case "k3d":
		return DriverK3d, nil
	default:
		return "", fmt.Errorf("unsupported cluster driver %q (use kind or k3d)", s)
	}
}

func (d Driver) String() string {
	if d == "" {
		return string(DriverKind)
	}
	return string(d)
}

func metaDriver(m Meta) Driver {
	switch Driver(m.Driver) {
	case DriverK3d:
		return DriverK3d
	default:
		return DriverKind
	}
}

func listRuntimeClusters(ctx context.Context, exec transport.Executor) (map[string]Driver, error) {
	result := map[string]Driver{}

	kindOut, err := inspect.RunOutput(ctx, exec, "kind get clusters 2>/dev/null || true")
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(strings.TrimSpace(kindOut), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			result[line] = DriverKind
		}
	}

	k3dOut, err := inspect.RunOutput(ctx, exec, "k3d cluster list 2>/dev/null | awk 'NR>1 && NF {print $1}' || true")
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(strings.TrimSpace(k3dOut), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			result[line] = DriverK3d
		}
	}
	return result, nil
}

func createCluster(ctx context.Context, exec transport.Executor, driver Driver, runtimeName string, workers, controlPlanes int, cfgPath string) (int, error) {
	if controlPlanes == 0 {
		controlPlanes = 1
	}
	var cmd string
	switch driver {
	case DriverK3d:
		cmd = fmt.Sprintf(
			"k3d cluster create %s --servers %d --agents %d --kubeconfig-update-default=false --wait",
			shellQuote(runtimeName), controlPlanes, workers,
		)
	default:
		cmd = fmt.Sprintf(
			"kind create cluster --name %s --config %s",
			shellQuote(runtimeName), shellQuote(cfgPath),
		)
	}
	return exec.Run(ctx, cmd, transport.RunOpts{})
}

func deleteCluster(ctx context.Context, exec transport.Executor, driver Driver, runtimeName string) error {
	var cmd string
	switch driver {
	case DriverK3d:
		cmd = fmt.Sprintf("k3d cluster delete %s 2>/dev/null || true", shellQuote(runtimeName))
	default:
		cmd = fmt.Sprintf("kind delete cluster --name %s 2>/dev/null || true", shellQuote(runtimeName))
	}
	_, err := exec.Run(ctx, cmd, transport.RunOpts{})
	return err
}

func fetchKubeconfig(ctx context.Context, exec transport.Executor, driver Driver, runtimeName string) (string, error) {
	var cmd string
	switch driver {
	case DriverK3d:
		cmd = fmt.Sprintf("k3d kubeconfig get %s", shellQuote(runtimeName))
	default:
		cmd = fmt.Sprintf("kind get kubeconfig --name %s", shellQuote(runtimeName))
	}
	return inspect.RunOutput(ctx, exec, cmd)
}

func nodeCountFor(ctx context.Context, exec transport.Executor, driver Driver, runtimeName string) (int, error) {
	var cmd string
	switch driver {
	case DriverK3d:
		cmd = fmt.Sprintf(
			"docker ps --filter label=k3d.cluster=%s --format '{{.Label \"k3d.role\"}}' | grep -E '^(server|agent)$' | wc -l",
			shellQuote(runtimeName),
		)
	default:
		cmd = fmt.Sprintf("docker ps --filter label=io.x-k8s.kind.cluster=%s --format '{{.ID}}' | wc -l", shellQuote(runtimeName))
	}
	out, err := inspect.RunOutput(ctx, exec, cmd)
	if err != nil {
		return 0, err
	}
	var n int
	fmt.Sscanf(strings.TrimSpace(out), "%d", &n)
	return n, nil
}

func resolveClusterTarget(ctx context.Context, exec transport.Executor, name string) (runtimeName string, driver Driver) {
	safe := config.SanitizeClusterName(name)
	runtimeName = KindName(name)
	driver = DriverKind
	if meta, err := loadMetaFromExec(exec, safe); err == nil {
		return meta.KindName, metaDriver(*meta)
	}
	if runtimeClusters, err := listRuntimeClusters(ctx, exec); err == nil {
		if drv, ok := runtimeClusters[runtimeName]; ok {
			return runtimeName, drv
		}
	}
	return runtimeName, driver
}

func loadMetaFromExec(exec transport.Executor, safeName string) (*Meta, error) {
	data, err := exec.Download(remoteBase + "/" + safeName + "/meta.yaml")
	if err != nil {
		return nil, err
	}
	var m Meta
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}
