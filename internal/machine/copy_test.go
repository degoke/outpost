package machine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/degoke/outpost/internal/machine"
	"github.com/stretchr/testify/require"
)

func TestParseCopyEndpointMachinePath(t *testing.T) {
	ep, ok, err := machine.ParseCopyEndpointForTest("ubuntu-dev:/tmp/foo.txt")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "ubuntu-dev", ep.Machine)
	require.Equal(t, "/tmp/foo.txt", ep.Path)
}

func TestParseCopyEndpointLocalPath(t *testing.T) {
	ep, ok, err := machine.ParseCopyEndpointForTest("./foo.txt")
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, "./foo.txt", ep.Path)
}

func TestExpandLocalPath(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, "dev/foo"), machine.ExpandLocalPathForTest("~/dev/foo"))
}

func TestIncusInstancePath(t *testing.T) {
	require.Equal(t, "outpost-ubuntu-dev/geass", machine.IncusInstancePathForTest("outpost-ubuntu-dev", "/geass"))
	require.Equal(t, "outpost-ubuntu-dev/tmp/foo", machine.IncusInstancePathForTest("outpost-ubuntu-dev", "/tmp/foo"))
}
