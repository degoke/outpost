package mirror_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/degoke/outpost/internal/mirror"
)

func TestWatchActiveCurrentProcess(t *testing.T) {
	dir := t.TempDir()
	release, err := mirror.AcquireWatchLockForTest("dev", "demo", dir)
	require.NoError(t, err)
	defer release()

	require.True(t, mirror.WatchActive("dev", "demo", dir))
	require.False(t, mirror.WatchActive("dev", "other", dir))
}
