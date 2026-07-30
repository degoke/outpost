package mirror_test

import (
	"testing"

	"github.com/degoke/outpost/internal/mirror"
	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/require"
)

func TestIsIgnorableWatchEvent(t *testing.T) {
	require.True(t, mirror.IsIgnorableWatchEventForTest(fsnotify.Event{Op: fsnotify.Chmod}))
	require.False(t, mirror.IsIgnorableWatchEventForTest(fsnotify.Event{Op: fsnotify.Write}))
	require.False(t, mirror.IsIgnorableWatchEventForTest(fsnotify.Event{Op: fsnotify.Write | fsnotify.Chmod}))
}
