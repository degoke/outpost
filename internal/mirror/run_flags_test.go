package mirror_test

import (
	"testing"

	"github.com/degoke/outpost/internal/mirror"
	"github.com/stretchr/testify/require"
)

func TestParseRunCLIArgs(t *testing.T) {
	flags, cmd, err := mirror.ParseRunCLIArgs([]string{"-d", "--", "echo", "hi"})
	require.NoError(t, err)
	require.True(t, flags.Detach)
	require.Equal(t, []string{"echo", "hi"}, cmd)

	flags, cmd, err = mirror.ParseRunCLIArgs([]string{"--name", "batch1", "python", "gen.py"})
	require.NoError(t, err)
	require.Equal(t, "batch1", flags.Name)
	require.Equal(t, []string{"python", "gen.py"}, cmd)

	flags, cmd, err = mirror.ParseRunCLIArgs([]string{"--attach", "batch1"})
	require.NoError(t, err)
	require.Equal(t, "batch1", flags.Attach)
	require.Empty(t, cmd)

	_, _, err = mirror.ParseRunCLIArgs([]string{"--attach", "a", "--detach"})
	require.Error(t, err)
}
