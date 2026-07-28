package cluster_test

import (
	"strings"
	"testing"

	"github.com/degoke/outpost/internal/cluster"
	"github.com/degoke/outpost/internal/config"
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
	require.Contains(t, cfg, "role: control-plane")
	require.NotContains(t, cfg, "role: worker")
}

func TestRenderKindConfigMultiNode(t *testing.T) {
	cfg := cluster.RenderKindConfig(cluster.KindConfig{Name: "outpost-x", ControlPlanes: 1, Workers: 2})
	require.Equal(t, 1, strings.Count(cfg, "role: control-plane"))
	require.Equal(t, 2, strings.Count(cfg, "role: worker"))
}

func TestEstimateResources(t *testing.T) {
	cpu, mem, disk := cluster.EstimateResources(1, 2)
	require.Equal(t, float64(4), cpu)
	require.Equal(t, uint64(4*1024*1024*1024), mem)
	require.Equal(t, uint64(5*1024*1024*1024), disk)
}
