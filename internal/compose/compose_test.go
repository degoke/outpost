package compose_test

import (
	"testing"

	"github.com/goke/outpost/internal/compose"
	"github.com/goke/outpost/internal/config"
	"github.com/stretchr/testify/require"
)

func TestBuildCommandStableProject(t *testing.T) {
	r := &compose.Runner{
		Project: &config.Project{
			Name:         "myapp",
			RemoteDir:    "/var/lib/outpost/projects/myapp",
			ComposeFiles: []string{"docker-compose.yml"},
		},
	}
	cmd := r.BuildCommand("up", []string{"-d"})
	require.Contains(t, cmd, "-p 'myapp'")
	require.Contains(t, cmd, "-f /var/lib/outpost/projects/myapp/docker-compose.yml")
	require.Contains(t, cmd, " up -d")
}

func TestIsDestructive(t *testing.T) {
	require.True(t, compose.IsDestructive("down", []string{}))
	require.True(t, compose.IsDestructive("down", []string{"-v"}))
	require.False(t, compose.IsDestructive("up", []string{"-d"}))
}
