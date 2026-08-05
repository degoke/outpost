package mirror_test

import (
	"context"
	"strings"
	"testing"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/mirror"
	"github.com/degoke/outpost/internal/testenv"
	"github.com/degoke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

func TestRunAttachSkipsSync(t *testing.T) {
	exec := mock.New()
	exec.Responses["command -v tmux >/dev/null 2>&1"] = mockOK()
	exec.Responses["tmux has-session -t outpost-demo-gen 2>/dev/null"] = mockExit(0)
	exec.Responses["tmux attach -t outpost-demo-gen"] = mockOK()
	exec.Responses[tailLogCmd("gen")] = mockStdout("running\n")

	proj := &config.Project{Name: "demo", RemoteDir: demoRemoteDir}
	runner := mirror.New(exec, proj, t.TempDir(), "personal", nil)
	_, err := runner.Run(context.Background(), mirror.RunOptions{
		AttachSession: "gen",
		ForceSync:     true,
	})
	require.NoError(t, err)
	for _, cmd := range exec.Commands {
		require.NotContains(t, strings.ToLower(cmd), "rsync")
	}
}

func TestRunAttachReturnsDetachedExitWhenStillRunning(t *testing.T) {
	exec := mock.New()
	exec.Responses["command -v tmux >/dev/null 2>&1"] = mockOK()
	exec.Responses["tmux has-session -t outpost-demo-gen 2>/dev/null"] = mockExit(0)
	exec.Responses["tmux attach -t outpost-demo-gen"] = mockOK()
	exec.Responses[tailLogCmd("gen")] = mockStdout("still going\n")

	proj := &config.Project{Name: "demo", RemoteDir: demoRemoteDir}
	runner := mirror.New(exec, proj, t.TempDir(), "personal", nil)
	result, err := runner.Run(context.Background(), mirror.RunOptions{AttachSession: "gen"})
	require.NoError(t, err)
	require.Equal(t, mirror.ExitSessionDetached, result.ExitCode)
	require.Equal(t, "gen", result.SessionName)
}

func TestRunAttachFinishedWithoutExitReturnsOne(t *testing.T) {
	exec := mock.New()
	exec.Responses["command -v tmux >/dev/null 2>&1"] = mockOK()
	exec.EnqueueResponse("tmux has-session -t outpost-demo-gen", 0, "")
	exec.EnqueueResponse("tmux has-session -t outpost-demo-gen", 1, "")
	exec.Responses["tmux attach -t outpost-demo-gen"] = mockOK()
	exec.Responses[tailLogCmd("gen")] = mockStdout("ended without marker\n")

	proj := &config.Project{Name: "demo", RemoteDir: demoRemoteDir}
	runner := mirror.New(exec, proj, t.TempDir(), "personal", nil)
	result, err := runner.Run(context.Background(), mirror.RunOptions{AttachSession: "gen"})
	require.NoError(t, err)
	require.Equal(t, 1, result.ExitCode)
}

func TestRunDetachedTruncatesLog(t *testing.T) {
	home := t.TempDir()
	testenv.UseHomeConfigDir(t, home)

	exec := mock.New()
	exec.Responses["command -v tmux >/dev/null 2>&1"] = mockOK()
	exec.Responses["tmux has-session -t outpost-demo-batch1 2>/dev/null"] = mockExit(1)
	exec.Responses["cd '"+demoRemoteDir+"' && mkdir -p "+demoRemoteDir+"/.outpost/sessions"] = mockOK()
	exec.Responses["cd '"+demoRemoteDir+"' && > "+demoRemoteDir+"/.outpost/sessions/batch1.log"] = mockOK()
	exec.Responses["tmux new-session -d -s outpost-demo-batch1 -c "+demoRemoteDir+" bash -lc"] = mockOK()

	proj := &config.Project{Name: "demo", RemoteDir: demoRemoteDir}
	runner := mirror.New(exec, proj, t.TempDir(), "personal", nil)
	result, err := runner.Run(context.Background(), mirror.RunOptions{
		Detach:      true,
		SessionName: "batch1",
		Command:     "echo hello",
		NoSync:      true,
	})
	require.NoError(t, err)
	require.Equal(t, "batch1", result.SessionName)

	foundTruncate := false
	for _, cmd := range exec.Commands {
		if strings.Contains(cmd, "> "+demoRemoteDir+"/.outpost/sessions/batch1.log") {
			foundTruncate = true
			break
		}
	}
	require.True(t, foundTruncate, "expected session log truncation before start")
}
