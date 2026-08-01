package cluster_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/degoke/outpost/internal/cluster"
	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/output"
	"github.com/degoke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestRewriteKubeconfigServer(t *testing.T) {
	input := []byte(`apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: CA
    server: https://127.0.0.1:43123
  name: outpost-demo
contexts: []
current-context: outpost-demo
kind: Config
users: []
`)
	out, err := cluster.RewriteKubeconfigServer(input, 51234)
	require.NoError(t, err)
	require.Contains(t, string(out), "server: https://127.0.0.1:51234")
	require.NotContains(t, string(out), "43123")
}

func TestRewriteKubeconfigRejectsMissingServer(t *testing.T) {
	_, err := cluster.RewriteKubeconfigServer([]byte("clusters: []\n"), 51234)
	require.Error(t, err)
}

func TestLocalProjectKubeconfigPath(t *testing.T) {
	cwd := t.TempDir()
	path := cluster.LocalProjectKubeconfigPath(cwd)
	require.Equal(t, filepath.Join(cwd, ".outpost", "kubeconfig"), path)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
}

func TestProjectUpRunsKubernetesInsideProjectContainer(t *testing.T) {
	dir := t.TempDir()
	project := &config.Project{Name: "demo", RemoteDir: "/var/lib/outpost/projects/demo", Environment: &config.ProjectEnvironment{}}
	exec := mock.New()
	exec.Responses["docker inspect 'outpost-dev-demo' >/dev/null 2>&1"] = struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{ExitCode: 1}
	exec.Responses["nproc"] = struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{Stdout: "4\n"}
	exec.Responses["free -b | head -2"] = struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{Stdout: "Mem: 8589934592 0 8589934592 0 0 8589934592\n"}
	exec.Responses["df -B1 / | tail -1"] = struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{Stdout: "/dev/root 100000000000 1000000000 99000000000 1% /\n"}
	exec.Responses["head -1 /proc/stat"] = struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{Stdout: "cpu  100 0 50 8500 0 0 0 0 0 0\n"}
	exec.Responses["docker stats --no-stream --format '{{json .}}'"] = struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{}
	exec.Responses["docker exec 'outpost-dev-demo' '/bin/bash' -lc 'kind get kubeconfig"] = struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{Stdout: `apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: CA
    server: https://0.0.0.0:43123
  name: outpost-demo
contexts: []
kind: Config
users: []
`}
	svc := cluster.NewProjectService(exec, project, dir, output.New(false, false))
	require.NoError(t, svc.Up(context.Background(), ""))
	require.True(t, exec.HasCommand("docker exec 'outpost-dev-demo' '/bin/bash' -lc 'kind create cluster"))
	require.False(t, exec.HasCommand("kind create cluster --name 'outpost-demo' --config"))
	data, err := os.ReadFile(config.ProjectConfigPath(dir))
	require.NoError(t, err)
	var saved config.Project
	require.NoError(t, yaml.Unmarshal(data, &saved))
	require.Equal(t, "kind", saved.Kubernetes.Driver)
}
