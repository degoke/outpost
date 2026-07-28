package connect_test

import (
	"context"
	"testing"

	"github.com/degoke/outpost/internal/connect"
	"github.com/degoke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

func TestDiscoverRemotePorts(t *testing.T) {
	exec := mock.New()
	exec.Responses["docker compose -p 'demo' -f /remote/docker-compose.yml ps --format json"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{
		Stdout: `[{"Service":"app","Publishers":[{"URL":"0.0.0.0","TargetPort":3000,"PublishedPort":3000,"Protocol":"tcp"}]},{"Service":"web","Publishers":[{"URL":"0.0.0.0","TargetPort":80,"PublishedPort":8080,"Protocol":"tcp"}]}]`,
	}

	mappings, err := connect.DiscoverRemotePorts(context.Background(), exec, "demo", "-f /remote/docker-compose.yml", "")
	require.NoError(t, err)
	require.Len(t, mappings, 2)
}

func TestDiscoverRemotePortsSingleService(t *testing.T) {
	exec := mock.New()
	exec.Responses["docker compose -p 'demo' -f /remote/docker-compose.yml ps --format json"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{
		Stdout: `[{"Service":"app","Publishers":[{"URL":"0.0.0.0","TargetPort":3000,"PublishedPort":3000,"Protocol":"tcp"}]},{"Service":"web","Publishers":[{"URL":"0.0.0.0","TargetPort":80,"PublishedPort":8080,"Protocol":"tcp"}]}]`,
	}

	mappings, err := connect.DiscoverRemotePorts(context.Background(), exec, "demo", "-f /remote/docker-compose.yml", "app")
	require.NoError(t, err)
	require.Len(t, mappings, 1)
	require.Equal(t, "app", mappings[0].Service)
}
