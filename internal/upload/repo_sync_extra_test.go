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

func TestIsSyncableRepoPathGitRespectsGitignore(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("a"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ignored.txt"), []byte("b"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("ignored.txt\n"), 0644))
	runGit(t, dir, "add", "tracked.txt", ".gitignore")
	runGit(t, dir, "commit", "-m", "init")

	require.True(t, upload.IsSyncableRepoPath(dir, "tracked.txt"))
	require.False(t, upload.IsSyncableRepoPath(dir, "ignored.txt"))
}

func TestCollectRepoPathsGitAlsoUsesOutpostignore(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".outpost"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("a"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "local-only.txt"), []byte("b"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("ignored.txt\n"), 0644))
	require.NoError(t, os.WriteFile(config.OutpostIgnorePath(dir), []byte("local-only.txt\n"), 0644))
	runGit(t, dir, "add", "tracked.txt", ".gitignore")
	runGit(t, dir, "commit", "-m", "init")

	paths, err := upload.CollectRepoPathsForTest(dir)
	require.NoError(t, err)
	require.Contains(t, paths, "tracked.txt")
	require.NotContains(t, paths, "local-only.txt")
}

func TestIsIgnoredByOutpost(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".outpost"), 0755))
	require.NoError(t, os.WriteFile(config.OutpostIgnorePath(dir), []byte("artifacts/\n"), 0644))
	require.True(t, upload.IsIgnoredByOutpost(dir, "artifacts/output.bin"))
	require.False(t, upload.IsIgnoredByOutpost(dir, "src/main.go"))
}

func TestSyncPathsRejectsRsyncMode(t *testing.T) {
	proj := &config.Project{Name: "demo", RemoteDir: "/var/lib/outpost/projects/demo"}
	err := upload.SyncPaths(t.TempDir(), proj, mock.New(), []string{"main.go"}, &upload.SyncOpts{UseRsync: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "partial path sync")
}
