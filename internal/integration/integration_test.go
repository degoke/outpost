package integration_test

import (
	"context"
	"strings"
	"testing"

	"github.com/goke/outpost/internal/compose"
	"github.com/goke/outpost/internal/config"
	"github.com/goke/outpost/internal/transport"
	"github.com/goke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

func TestComposeUpTwiceSameProject(t *testing.T) {
	exec := mock.New()
	proj := &config.Project{
		Name:         "demo",
		RemoteDir:    "/var/lib/outpost/projects/demo",
		ComposeFiles: []string{"docker-compose.yml"},
	}
	runner := &compose.Runner{Exec: exec, Project: proj, Cwd: t.TempDir()}

	ctx := context.Background()
	_, err := runner.Run(ctx, "up", []string{"-d"}, false)
	require.NoError(t, err)
	first := exec.LastCommand()
	_, err = runner.Run(ctx, "up", []string{"-d"}, false)
	require.NoError(t, err)
	second := exec.LastCommand()
	require.Equal(t, first, second)
	require.True(t, strings.Contains(first, "-p 'demo'"))
}

func TestMockSSHFailure(t *testing.T) {
	exec := mock.New()
	exec.Responses["false"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{ExitCode: 1, Stderr: "permission denied"}
	code, err := exec.Run(context.Background(), "false", transport.RunOpts{})
	require.NoError(t, err)
	require.Equal(t, 1, code)
}
