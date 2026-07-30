package cluster_test

import (
	"context"
	"testing"

	"github.com/degoke/outpost/internal/cluster"
	"github.com/degoke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

func TestDeleteWithoutMetaUsesK3dRuntime(t *testing.T) {
	exec := mock.New()
	exec.Responses["kind get clusters 2>/dev/null || true"] = struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{Stdout: "", ExitCode: 0}
	exec.Responses["k3d cluster list 2>/dev/null | awk 'NR>1 && NF {print $1}' || true"] = struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{Stdout: "outpost-dev\n", ExitCode: 0}
	exec.Responses["k3d cluster delete"] = struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{ExitCode: 0}
	exec.Responses["rm -rf"] = struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{ExitCode: 0}

	svc := &cluster.Service{Exec: exec}
	err := svc.Delete(context.Background(), "dev")
	require.NoError(t, err)
	require.True(t, exec.HasCommand("k3d cluster delete"))
	require.False(t, exec.HasCommand("kind delete cluster"))
}

func TestListIncludesK3dOrphanCluster(t *testing.T) {
	exec := mock.New()
	exec.Responses["kind get clusters 2>/dev/null || true"] = struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{Stdout: "", ExitCode: 0}
	exec.Responses["k3d cluster list 2>/dev/null | awk 'NR>1 && NF {print $1}' || true"] = struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{Stdout: "outpost-staging\n", ExitCode: 0}
	exec.Responses["ls -1 /var/lib/outpost/clusters 2>/dev/null || true"] = struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{Stdout: "", ExitCode: 0}
	exec.Responses["docker ps --filter label=k3d.cluster="] = struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{Stdout: "1\n", ExitCode: 0}

	svc := &cluster.Service{Exec: exec}
	list, err := svc.List(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "staging", list[0].Name)
	require.Equal(t, "k3d", list[0].Driver)
	require.Equal(t, "ready", list[0].Status)
}
