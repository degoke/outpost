package cluster_test

import (
	"strings"
	"testing"

	"github.com/degoke/outpost/internal/capacity"
	"github.com/degoke/outpost/internal/cluster"
	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/inspect"
	"github.com/stretchr/testify/require"
)

func TestSanitizeClusterNameMatchesProject(t *testing.T) {
	require.Equal(t, config.SanitizeProjectName("My Cluster"), config.SanitizeClusterName("My Cluster"))
}

func TestKindNamePrefix(t *testing.T) {
	require.Equal(t, "outpost-demo", cluster.KindName("demo"))
	require.Equal(t, "outpost-my-app", cluster.KindName("my app"))
}

func TestRenderKindConfigSingleNode(t *testing.T) {
	cfg := cluster.RenderKindConfig(cluster.KindConfig{Name: "outpost-demo", ControlPlanes: 1})
	require.Contains(t, cfg, "name: outpost-demo")
	require.Contains(t, cfg, "apiServerAddress: 0.0.0.0")
	require.Contains(t, cfg, "host.docker.internal")
	require.Contains(t, cfg, "127.0.0.1")
	require.Contains(t, cfg, "role: control-plane")
	require.NotContains(t, cfg, "role: worker")
}

func TestRenderKindConfigMultiNode(t *testing.T) {
	cfg := cluster.RenderKindConfig(cluster.KindConfig{Name: "outpost-x", ControlPlanes: 1, Workers: 2})
	require.Equal(t, 1, strings.Count(cfg, "role: control-plane"))
	require.Equal(t, 2, strings.Count(cfg, "role: worker"))
}

func TestEstimateResourcesKind(t *testing.T) {
	cpu, mem, disk := cluster.EstimateResources(cluster.DriverKind, 1, 2)
	require.Equal(t, 2.75, cpu)
	require.Equal(t, uint64(4*1024*1024*1024), mem)
	require.Equal(t, uint64(2560*1024*1024), disk)
}

func TestEstimateResourcesK3dSingleNode(t *testing.T) {
	cpu, mem, disk := cluster.EstimateResources(cluster.DriverK3d, 1, 0)
	require.Equal(t, 0.75, cpu)
	require.Equal(t, uint64(768*1024*1024), mem)
	require.Equal(t, uint64(768*1024*1024), disk)
}

func TestEstimateResourcesFitsTwoCoreDevHost(t *testing.T) {
	const gib = 1024 * 1024 * 1024
	rep := &capacity.Report{
		Host:          inspect.HostMetrics{CPUCores: 2, MemoryTotal: 4 * gib},
		AvailableCPU:  1.8, // 2 cores minus 10% capacity margin
		AvailableMem:  3 * gib,
		AvailableDisk: 20 * gib,
	}

	k3dCPU, k3dMem, k3dDisk := cluster.EstimateResources(cluster.DriverK3d, 1, 0)
	require.NoError(t, capacity.CheckWithReport(rep, capacity.Request{
		CPUCores: k3dCPU, MemoryBytes: k3dMem, DiskBytes: k3dDisk,
	}))

	kindCPU, kindMem, kindDisk := cluster.EstimateResources(cluster.DriverKind, 1, 0)
	require.NoError(t, capacity.CheckWithReport(rep, capacity.Request{
		CPUCores: kindCPU, MemoryBytes: kindMem, DiskBytes: kindDisk,
	}))
}
