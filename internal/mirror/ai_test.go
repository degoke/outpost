package mirror_test

import (
	"context"
	"testing"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/mirror"
	"github.com/degoke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

func TestAIRequiresManagedEnvironment(t *testing.T) {
	enabled := false
	proj := &config.Project{
		Name:      "demo",
		RemoteDir: "/var/lib/outpost/projects/demo",
		Environment: &config.ProjectEnvironment{
			Enabled: &enabled,
		},
	}
	runner := mirror.New(mock.New(), proj, t.TempDir(), "dev", nil)
	code, err := runner.AI(context.Background(), mirror.AIOptions{})
	require.Error(t, err)
	require.Equal(t, 1, code)
	require.Contains(t, err.Error(), "managed development environment")
}

func TestResolveAICommandUsesOverrideAndProjectConfig(t *testing.T) {
	exec := mock.New()
	proj := &config.Project{
		Name:        "demo",
		RemoteDir:   "/var/lib/outpost/projects/demo",
		Environment: &config.ProjectEnvironment{Image: "outpost-dev:demo"},
		AI:          &config.ProjectAI{Command: "claude"},
	}
	runner := mirror.New(exec, proj, t.TempDir(), "dev", nil)

	cmd, err := runner.ResolveAICommandForTest(context.Background(), "")
	require.NoError(t, err)
	require.Equal(t, "claude", cmd)

	cmd, err = runner.ResolveAICommandForTest(context.Background(), "opencode --model anthropic/claude")
	require.NoError(t, err)
	require.Equal(t, "opencode --model anthropic/claude", cmd)
}

func TestResolveAICommandMissingAgent(t *testing.T) {
	exec := mock.New()
	exec.Responses["docker exec"] = mockResp(1, "")
	proj := &config.Project{
		Name:        "demo",
		RemoteDir:   "/var/lib/outpost/projects/demo",
		Environment: &config.ProjectEnvironment{Image: "outpost-dev:demo"},
	}
	runner := mirror.New(exec, proj, t.TempDir(), "dev", nil)

	_, err := runner.ResolveAICommandForTest(context.Background(), "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no AI agent found")
}

func mockResp(code int, stdout string) struct {
	Stdout, Stderr string
	ExitCode       int
	Err            error
} {
	return struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{Stdout: stdout, ExitCode: code}
}
