package app_test

import (
	"context"
	"testing"

	appsvc "github.com/degoke/outpost/internal/app"
	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/output"
	"github.com/degoke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

func TestBuildUsesProjectRemoteDirectoryAndImage(t *testing.T) {
	exec := mock.New()
	runner := &appsvc.Runner{
		Exec:    exec,
		Project: &config.Project{Name: "my-api", RemoteDir: "/var/lib/outpost/projects/my-api"},
		Out:     output.New(true, false),
	}

	require.NoError(t, runner.Build(context.Background(), "Dockerfile", "."))
	require.Contains(t, exec.LastCommand(), "cd '/var/lib/outpost/projects/my-api'")
	require.Contains(t, exec.LastCommand(), "docker build -t 'outpost-app-my-api'")
}

func TestDetachedRunUsesProjectContainerName(t *testing.T) {
	exec := mock.New()
	runner := &appsvc.Runner{
		Exec:    exec,
		Project: &config.Project{Name: "my-api"},
		Out:     output.New(true, false),
	}

	code, err := runner.Run(context.Background(), []string{"8080:8080"}, true, []string{"serve", "--debug"})
	require.NoError(t, err)
	require.Zero(t, code)
	require.Equal(t, "docker run --name 'outpost-app-my-api' -p '8080:8080' -d 'outpost-app-my-api' 'serve' '--debug'", exec.LastCommand())
}
