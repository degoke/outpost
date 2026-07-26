package upload_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goke/outpost/internal/config"
	"github.com/goke/outpost/internal/transport/mock"
	"github.com/goke/outpost/internal/upload"
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
	require.NoError(t, upload.SyncProject(dir, proj, exec))
	require.Contains(t, exec.Uploads, "/var/lib/outpost/projects/demo/docker-compose.yml")
}

func TestRemoteComposeArgs(t *testing.T) {
	proj := &config.Project{
		RemoteDir:    "/var/lib/outpost/projects/demo",
		ComposeFiles: []string{"docker-compose.yml"},
	}
	args := upload.RemoteComposeArgs(proj)
	require.Contains(t, args, "-f /var/lib/outpost/projects/demo/docker-compose.yml")
}
