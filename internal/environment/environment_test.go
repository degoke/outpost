package environment_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/environment"
	"github.com/degoke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

func TestEnsureCreatesManagedProjectContainer(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"demo"}`), 0o644))
	exec := mock.New()
	exec.Responses["docker inspect"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{ExitCode: 1}
	project := &config.Project{Name: "demo", RemoteDir: "/var/lib/outpost/projects/demo", Environment: &config.ProjectEnvironment{}}
	require.NoError(t, environment.New(exec, project, dir).Ensure(context.Background()))
	require.True(t, exec.HasCommand("--label 'com.outpost.managed=true'"))
	require.True(t, exec.HasCommand("docker build -t 'outpost-dev-demo:auto'"))
	require.True(t, exec.HasCommand("outpost-dev-demo:auto"))
	require.True(t, exec.HasCommand("outpost-deps-demo-node:/workspace/node_modules"))
}

func TestEnsureBuildsCombinedImageFromNestedManifests(t *testing.T) {
	dir := t.TempDir()
	uiDir := filepath.Join(dir, "ui")
	require.NoError(t, os.MkdirAll(uiDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/demo\n\ngo 1.26.2\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(uiDir, "package.json"), []byte(`{
  "packageManager": "pnpm@10.29.3"
}`), 0o644))
	exec := mock.New()
	exec.Responses["docker inspect"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{ExitCode: 1}
	project := &config.Project{Name: "demo", RemoteDir: "/var/lib/outpost/projects/demo", Environment: &config.ProjectEnvironment{}}
	require.NoError(t, environment.New(exec, project, dir).Ensure(context.Background()))
	require.True(t, exec.HasCommand("docker build -t 'outpost-dev-demo:auto'"))
	require.True(t, exec.HasCommand("outpost-dev-demo:auto"))
	require.True(t, exec.HasCommand("--label 'com.outpost.managed=true'"))
}

func TestDevcontainerImageAndWorkspaceAreHonored(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".devcontainer"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".devcontainer", "devcontainer.json"), []byte(`{
  // JSONC is accepted like standard Dev Containers.
  "image": "ghcr.io/example/dev:latest",
  "workspaceFolder": "/workspaces/demo",
  "forwardPorts": ["3000:3000"]
}`), 0o644))
	exec := mock.New()
	exec.Responses["docker inspect"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{ExitCode: 1}
	project := &config.Project{Name: "demo", RemoteDir: "/var/lib/outpost/projects/demo", Environment: &config.ProjectEnvironment{}}
	require.NoError(t, environment.New(exec, project, dir).Ensure(context.Background()))
	require.True(t, exec.HasCommand("ghcr.io/example/dev:latest"))
	require.True(t, exec.HasCommand("--workdir '/workspaces/demo'"))
	require.Equal(t, []int{3000}, project.Environment.Ports)
}
