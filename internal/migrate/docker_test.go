package migrate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/migrate"
	"github.com/stretchr/testify/require"
)

func TestRemoveLocalBundleDeletesStaleArchive(t *testing.T) {
	dir, err := config.VolumeArchivesDir("demo")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(dir, 0o700))
	stale := filepath.Join(dir, "docker-bundle.tar")
	require.NoError(t, os.WriteFile(stale, []byte("stale"), 0o600))

	migrate.RemoveLocalBundleForTest("demo")

	_, err = os.Stat(stale)
	require.True(t, os.IsNotExist(err))
}
