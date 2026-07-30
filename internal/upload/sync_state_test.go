package upload_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/degoke/outpost/internal/upload"
)

func TestNeedsRepoSyncSkipsWhenUnchanged(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644))
	require.NoError(t, upload.MarkRepoSynced(dir, "dev", "demo"))

	needs, err := upload.NeedsRepoSync(dir, "dev", "demo")
	require.NoError(t, err)
	require.False(t, needs)
}

func TestNeedsRepoSyncDetectsChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	require.NoError(t, os.WriteFile(path, []byte("v1"), 0644))
	require.NoError(t, upload.MarkRepoSynced(dir, "dev", "demo"))

	require.NoError(t, os.WriteFile(path, []byte("v2"), 0644))
	needs, err := upload.NeedsRepoSync(dir, "dev", "demo")
	require.NoError(t, err)
	require.True(t, needs)
}
