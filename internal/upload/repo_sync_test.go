package upload_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/goke/outpost/internal/config"
	"github.com/goke/outpost/internal/transport/mock"
	"github.com/goke/outpost/internal/upload"
	"github.com/stretchr/testify/require"
)

func TestCollectRepoPathsGitRespectsGitignore(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("a"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ignored.txt"), []byte("b"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("ignored.txt\n.venv/\n"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".venv", "bin"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".venv", "bin", "python"), []byte("bin"), 0755))
	runGit(t, dir, "add", "tracked.txt", ".gitignore")
	runGit(t, dir, "commit", "-m", "init")

	paths, err := upload.CollectRepoPathsForTest(dir)
	require.NoError(t, err)
	require.Contains(t, paths, "tracked.txt")
	require.Contains(t, paths, ".gitignore")
	require.NotContains(t, paths, "ignored.txt")
	require.NotContains(t, paths, ".venv/bin/python")
}

func TestCollectRepoPathsWalkUsesOutpostignore(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("a"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "skip"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "skip", "drop.txt"), []byte("b"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".outpostignore"), []byte("skip/\n"), 0644))

	paths, err := upload.CollectRepoPathsForTest(dir)
	require.NoError(t, err)
	require.Contains(t, paths, "keep.txt")
	require.Contains(t, paths, ".outpostignore")
	require.NotContains(t, paths, "skip/drop.txt")
}

func TestSyncRepoUploadsFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "script.py"), []byte("print('ok')"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".outpostignore"), []byte(""), 0644))

	proj := &config.Project{
		Name:      "demo",
		RemoteDir: "/var/lib/outpost/projects/demo",
	}
	exec := mock.New()
	require.NoError(t, upload.SyncRepo(dir, proj, exec))
	require.Contains(t, exec.Uploads, "/var/lib/outpost/projects/demo/script.py")
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}
