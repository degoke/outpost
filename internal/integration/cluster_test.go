package integration_test

import (
	"context"
	"testing"

	"github.com/degoke/outpost/internal/cluster"
	"github.com/degoke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

func TestClusterListIntegration(t *testing.T) {
	exec := mock.New()
	exec.Responses["kind get clusters 2>/dev/null || true"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: "outpost-dev\noutpost-staging\n", ExitCode: 0}
	exec.Responses["ls -1 /var/lib/outpost/clusters 2>/dev/null || true"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: "dev\n", ExitCode: 0}
	exec.Files["/var/lib/outpost/clusters/dev/meta.yaml"] = []byte(`name: dev
kind_name: outpost-dev
workers: 1
control_planes: 1
`)
	exec.Responses["docker ps --filter label=io.x-k8s.kind.cluster="] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: "2\n", ExitCode: 0}

	svc := &cluster.Service{Exec: exec, HostName: "personal"}
	list, err := svc.List(context.Background())
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(list), 2)
}

func TestKubectlCommandConstruction(t *testing.T) {
	exec := mock.New()
	args := []string{"get", "nodes"}
	code, err := cluster.RunKubectl(context.Background(), exec, "dev", args)
	require.NoError(t, err)
	require.Equal(t, 0, code)
	require.True(t, exec.HasCommand("KUBECONFIG="))
	require.True(t, exec.HasCommand("kubectl"))
}
