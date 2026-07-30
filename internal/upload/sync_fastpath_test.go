package upload_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/degoke/outpost/internal/upload"
	"github.com/stretchr/testify/require"
)

func TestModTimesEqual(t *testing.T) {
	now := time.Date(2026, 1, 15, 12, 30, 45, 123456789, time.UTC)
	local := fakeFileInfo{size: 10, mod: now}
	remote := fakeFileInfo{size: 10, mod: now.Add(500 * time.Millisecond)}
	require.True(t, upload.ModTimesEqual(local, remote))
}

func TestModTimesEqualDifferentSecond(t *testing.T) {
	local := fakeFileInfo{size: 10, mod: time.Unix(100, 0)}
	remote := fakeFileInfo{size: 10, mod: time.Unix(101, 0)}
	require.False(t, upload.ModTimesEqual(local, remote))
}

func TestShouldIgnoreRel(t *testing.T) {
	patterns := upload.AlwaysIgnorePatterns()
	require.True(t, upload.ShouldIgnoreRel(".git/config", patterns, false))
	require.True(t, upload.ShouldIgnoreRel(".outpost/state", patterns, false))
	require.False(t, upload.ShouldIgnoreRel("src/main.go", patterns, false))
}

type fakeFileInfo struct {
	size int64
	mod  time.Time
}

func (f fakeFileInfo) Name() string       { return "file" }
func (f fakeFileInfo) Size() int64        { return f.size }
func (f fakeFileInfo) Mode() os.FileMode  { return 0644 }
func (f fakeFileInfo) ModTime() time.Time { return f.mod }
func (f fakeFileInfo) IsDir() bool        { return false }
func (f fakeFileInfo) Sys() any           { return nil }

func TestQuoteSSHArgs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "my key")
	quoted := upload.QuoteSSHArgsForTest([]string{"-i", path})
	require.Equal(t, "'"+path+"'", quoted[1])
}
