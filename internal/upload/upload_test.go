package upload_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/transport/mock"
	"github.com/degoke/outpost/internal/upload"
	"github.com/stretchr/testify/require"
)

func TestSyncProjectUploadsCompose(t *testing.T) {
	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	require.NoError(t, os.WriteFile(composePath, []byte("services: {}"), 0644))
	proj := &config.Project{
		Name:         "demo",
		RemoteDir:    "/var/lib/outpost/projects/demo",
		ComposeFiles: []string{"docker-compose.yml"},
	}
	exec := mock.New()
	require.NoError(t, upload.SyncProject(dir, proj, exec, nil))
	require.Contains(t, exec.Uploads, "/var/lib/outpost/projects/demo/docker-compose.yml")
}

func TestSyncProjectUploadsBuildContext(t *testing.T) {
	dir := t.TempDir()
	compose := `
services:
  app:
    build: .
    image: demo-app
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(compose), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM scratch"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "index.js"), []byte("console.log('ok')"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"demo"}`), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "node_modules", "left-pad"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "node_modules", "left-pad", "index.js"), []byte("module.exports = () => {}"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".dockerignore"), []byte("node_modules\n"), 0644))

	proj := &config.Project{
		Name:         "demo",
		RemoteDir:    "/var/lib/outpost/projects/demo",
		ComposeFiles: []string{"docker-compose.yml"},
	}
	exec := mock.New()
	require.NoError(t, upload.SyncProject(dir, proj, exec, nil))

	require.Contains(t, exec.Uploads, "/var/lib/outpost/projects/demo/docker-compose.yml")
	require.Contains(t, exec.Uploads, "/var/lib/outpost/projects/demo/Dockerfile")
	require.Contains(t, exec.Uploads, "/var/lib/outpost/projects/demo/src/index.js")
	require.Contains(t, exec.Uploads, "/var/lib/outpost/projects/demo/package.json")
	require.NotContains(t, exec.Uploads, "/var/lib/outpost/projects/demo/node_modules/left-pad/index.js")
}

func TestCollectSyncPathsNestedBuildContext(t *testing.T) {
	dir := t.TempDir()
	compose := `
services:
  api:
    build:
      context: ./backend
      dockerfile: Dockerfile
`
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "backend"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(compose), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "backend", "Dockerfile"), []byte("FROM scratch"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "backend", "main.go"), []byte("package main"), 0644))

	proj := &config.Project{
		ComposeFiles: []string{"docker-compose.yml"},
	}
	paths, err := upload.CollectSyncPathsForTest(dir, proj)
	require.NoError(t, err)
	require.Contains(t, paths, "docker-compose.yml")
	require.Contains(t, paths, "backend/Dockerfile")
	require.Contains(t, paths, "backend/main.go")
}

func TestRemoteComposeArgs(t *testing.T) {
	proj := &config.Project{
		RemoteDir:    "/var/lib/outpost/projects/demo",
		ComposeFiles: []string{"docker-compose.yml"},
		ExtraFiles:   []string{"docker-compose.override.yml"},
	}
	args := upload.RemoteComposeArgs(proj)
	require.Contains(t, args, "-f /var/lib/outpost/projects/demo/docker-compose.yml")
	require.Contains(t, args, "-f /var/lib/outpost/projects/demo/docker-compose.override.yml")
}
