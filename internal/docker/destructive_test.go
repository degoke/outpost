package docker_test

import (
	"testing"

	"github.com/degoke/outpost/internal/docker"
	"github.com/stretchr/testify/require"
)

func TestIsDestructive(t *testing.T) {
	require.True(t, docker.IsDestructive([]string{"rm", "-f", "abc"}))
	require.True(t, docker.IsDestructive([]string{"system", "prune"}))
	require.True(t, docker.IsDestructive([]string{"volume", "rm", "data"}))
	require.True(t, docker.IsDestructive([]string{"compose", "down"}))
	require.False(t, docker.IsDestructive([]string{"ps"}))
	require.False(t, docker.IsDestructive([]string{"images"}))
}
