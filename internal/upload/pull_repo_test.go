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

func TestPullRepoLegacyDownloadsChangedFiles(t *testing.T) {
	dir := t.TempDir()
	localPath := filepath.Join(dir, "main.go")
	require.NoError(t, os.WriteFile(localPath, []byte("local"), 0644))

	proj := &config.Project{
		Name:      "demo",
		RemoteDir: "/var/lib/outpost/projects/demo",
	}
	exec := mock.New()
	exec.Files["/var/lib/outpost/projects/demo/main.go"] = []byte("remote")

	require.NoError(t, upload.PullRepo(dir, proj, exec, nil))

	data, err := os.ReadFile(localPath)
	require.NoError(t, err)
	require.Equal(t, "remote", string(data))
}

func TestPullRepoLegacySkipsUnchangedFiles(t *testing.T) {
	dir := t.TempDir()
	localPath := filepath.Join(dir, "main.go")
	require.NoError(t, os.WriteFile(localPath, []byte("same"), 0644))

	proj := &config.Project{
		Name:      "demo",
		RemoteDir: "/var/lib/outpost/projects/demo",
	}
	exec := mock.New()
	exec.Files["/var/lib/outpost/projects/demo/main.go"] = []byte("same")

	infoBefore, err := os.Stat(localPath)
	require.NoError(t, err)

	require.NoError(t, upload.PullRepo(dir, proj, exec, nil))

	infoAfter, err := os.Stat(localPath)
	require.NoError(t, err)
	require.Equal(t, infoBefore.ModTime(), infoAfter.ModTime())
}

func TestPullRepoLegacyDownloadsNewRemoteOnlyFile(t *testing.T) {
	dir := t.TempDir()

	proj := &config.Project{
		Name:      "demo",
		RemoteDir: "/var/lib/outpost/projects/demo",
	}
	exec := mock.New()
	exec.Responses["find "] = mockResp(0, "agent-created.go\n")
	exec.Files["/var/lib/outpost/projects/demo/agent-created.go"] = []byte("package main")

	require.NoError(t, upload.PullRepo(dir, proj, exec, nil))

	data, err := os.ReadFile(filepath.Join(dir, "agent-created.go"))
	require.NoError(t, err)
	require.Equal(t, "package main", string(data))
}

func mockResp(code int, stdout string) struct {
	Stdout, Stderr string
	ExitCode       int
	Err            error
} {
	return struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{Stdout: stdout, ExitCode: code}
}
