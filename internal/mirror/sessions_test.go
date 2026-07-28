package mirror_test

import (
	"context"
	"strings"
	"testing"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/mirror"
	"github.com/degoke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

func TestListSessionsFiltersProjectPrefix(t *testing.T) {
	exec := mock.New()
	exec.Responses["tmux list-sessions -F '#{session_name}' 2>/dev/null || true"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{
		Stdout: "outpost-demo-gen\nother-session\noutpost-demo-build\n",
	}
	exec.Responses["tmux has-session -t 'outpost-demo-gen' 2>/dev/null"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{ExitCode: 0}
	exec.Responses["tmux has-session -t 'outpost-demo-build' 2>/dev/null"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{ExitCode: 1}
	exec.Responses["tail -n 50 '/var/lib/outpost/projects/demo/.outpost/sessions/gen.log' 2>/dev/null || true"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: "done\nEXIT:0\n"}
	exec.Responses["tail -n 50 '/var/lib/outpost/projects/demo/.outpost/sessions/build.log' 2>/dev/null || true"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: "running\n"}

	proj := &config.Project{Name: "demo", RemoteDir: "/var/lib/outpost/projects/demo"}
	runner := mirror.New(exec, proj, t.TempDir(), "personal")
	sessions, err := runner.ListSessions(context.Background())
	require.NoError(t, err)
	require.Len(t, sessions, 2)
	names := []string{sessions[0].Name, sessions[1].Name}
	require.Contains(t, names, "gen")
	require.Contains(t, names, "build")
}

func TestParseExitLineViaStatus(t *testing.T) {
	exec := mock.New()
	remoteDir := "/var/lib/outpost/projects/demo"
	logPath := remoteDir + "/.outpost/sessions/gen.log"
	exec.Responses["tmux has-session -t 'outpost-demo-gen' 2>/dev/null"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{ExitCode: 1}
	logTail := strings.Join([]string{"line1", "EXIT:42"}, "\n")
	tailCmd := "cd '" + remoteDir + "' && tail -n 50 " + logPath + " 2>/dev/null || true"
	exec.Responses[tailCmd] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: logTail}

	proj := &config.Project{Name: "demo", RemoteDir: remoteDir}
	runner := mirror.New(exec, proj, t.TempDir(), "personal")
	status, err := runner.SessionStatus(context.Background(), "gen")
	require.NoError(t, err)
	require.NotNil(t, status.ExitCode)
	require.Equal(t, 42, *status.ExitCode)
}
