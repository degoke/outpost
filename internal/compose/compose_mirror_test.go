package compose_test

import (
	"context"
	"testing"

	"github.com/goke/outpost/internal/compose"
	"github.com/goke/outpost/internal/config"
	"github.com/goke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

func TestRunnerRequiresComposeFiles(t *testing.T) {
	runner := &compose.Runner{
		Exec:    mock.New(),
		Project: &config.Project{Name: "demo", RemoteDir: "/var/lib/outpost/projects/demo"},
	}
	code, err := runner.Run(context.Background(), "ps", nil, false)
	require.Error(t, err)
	require.Equal(t, 1, code)
	require.Contains(t, err.Error(), "no compose files configured")
}
