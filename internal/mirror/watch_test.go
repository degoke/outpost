package mirror_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/degoke/outpost/internal/mirror"
	"github.com/stretchr/testify/require"
)

func TestRepoRelativePath(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "src", "main.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(file), 0755))
	require.NoError(t, os.WriteFile(file, []byte("ok"), 0644))

	rel, ok := mirror.RepoRelativePathForTest(root, file)
	require.True(t, ok)
	require.Equal(t, "src/main.go", rel)
}

func TestShouldWatchRelSkipsIgnored(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0755))
	require.False(t, mirror.ShouldWatchRelForTest(root, ".git/config"))
}
