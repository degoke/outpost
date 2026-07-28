package cluster_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/degoke/outpost/internal/cluster"
	"github.com/degoke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

func TestResolveLocalFilesUploadsManifest(t *testing.T) {
	exec := mock.New()
	dir := t.TempDir()
	manifest := filepath.Join(dir, "deploy.yaml")
	require.NoError(t, os.WriteFile(manifest, []byte("apiVersion: v1\n"), 0644))

	args := []string{"apply", "-f", manifest}
	resolved, err := cluster.ResolveLocalFilesForTest(args, "/var/lib/outpost/clusters/demo/uploads", exec)
	require.NoError(t, err)
	require.Equal(t, "apply", resolved[0])
	require.Equal(t, "-f", resolved[1])
	require.Contains(t, resolved[2], "/var/lib/outpost/clusters/demo/uploads/deploy.yaml")
	require.NotEmpty(t, exec.Uploads)
}

func TestListClustersFromMock(t *testing.T) {
	exec := mock.New()
	exec.Responses["kind get clusters 2>/dev/null || true"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: "outpost-demo\n", ExitCode: 0}
	exec.Responses["ls -1 /var/lib/outpost/clusters 2>/dev/null || true"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: "", ExitCode: 0}
	exec.Responses["docker ps --filter label=io.x-k8s.kind.cluster="] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: "1\n", ExitCode: 0}

	svc := &cluster.Service{Exec: exec}
	list, err := svc.List(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "demo", list[0].Name)
	require.Equal(t, "ready", list[0].Status)
}
