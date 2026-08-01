package docker_test

import (
	"context"
	"testing"

	"github.com/degoke/outpost/internal/docker"
	"github.com/degoke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

func TestRunStripsChildTTYForInteractiveDocker(t *testing.T) {
	exec := mock.New()
	_, err := docker.Run(context.Background(), exec, []string{"run", "-it", "ubuntu"})
	require.NoError(t, err)
	require.True(t, exec.HasCommand("docker run -i ubuntu"))
	require.False(t, exec.HasCommand("docker run -it ubuntu"))
}
