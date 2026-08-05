package mirror_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/mirror"
	"github.com/degoke/outpost/internal/testenv"
	"github.com/degoke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

const demoRemoteDir = "/var/lib/outpost/projects/demo"

func TestListSessionsFiltersProjectPrefix(t *testing.T) {
	exec := mock.New()
	exec.Responses["command -v tmux >/dev/null 2>&1"] = mockOK()
	exec.Responses["tmux list-sessions -F '#{session_name}' 2>/dev/null || true"] = mockStdout("outpost-demo-gen\nother-session\noutpost-demo-build\n")
	exec.Responses["tmux has-session -t outpost-demo-gen 2>/dev/null"] = mockExit(0)
	exec.Responses["tmux has-session -t outpost-demo-build 2>/dev/null"] = mockExit(1)
	exec.Responses[tailLogCmd("gen")] = mockStdout("done\nEXIT:0\n")
	exec.Responses[tailLogCmd("build")] = mockStdout("running\n")

	proj := &config.Project{Name: "demo", RemoteDir: demoRemoteDir}
	runner := mirror.New(exec, proj, t.TempDir(), "personal", nil)
	sessions, err := runner.ListSessions(context.Background())
	require.NoError(t, err)
	require.Len(t, sessions, 2)
	names := []string{sessions[0].Name, sessions[1].Name}
	require.Contains(t, names, "gen")
	require.Contains(t, names, "build")
}

func TestListSessionsIncludesFinishedFromMeta(t *testing.T) {
	home := t.TempDir()
	testenv.UseHomeConfigDir(t, home)

	require.NoError(t, mirror.SaveSessionMeta(mirror.SessionMeta{
		Host:      "personal",
		Project:   "demo",
		Name:      "batch.old",
		TmuxName:  "outpost-demo-batch.old",
		Command:   "python train.py",
		StartedAt: time.Now().UTC().Add(-24 * time.Hour),
	}))

	exec := mock.New()
	exec.Responses["command -v tmux >/dev/null 2>&1"] = mockOK()
	exec.Responses["tmux list-sessions -F '#{session_name}' 2>/dev/null || true"] = mockStdout("")
	exec.Responses["tmux has-session -t outpost-demo-batch.old 2>/dev/null"] = mockExit(1)
	exec.Responses[tailLogCmd("batch.old")] = mockStdout("done\nEXIT:0\n")

	proj := &config.Project{Name: "demo", RemoteDir: demoRemoteDir}
	runner := mirror.New(exec, proj, t.TempDir(), "personal", nil)
	sessions, err := runner.ListSessions(context.Background())
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.Equal(t, "batch.old", sessions[0].Name)
	require.False(t, sessions[0].Running)
	require.NotNil(t, sessions[0].ExitCode)
	require.Equal(t, 0, *sessions[0].ExitCode)
	require.Equal(t, "python train.py", sessions[0].Command)
}

func TestSessionMetaRoundTripAllowsDots(t *testing.T) {
	home := t.TempDir()
	testenv.UseHomeConfigDir(t, home)

	meta := mirror.SessionMeta{
		Host:      "personal",
		Project:   "demo",
		Name:      "gen.batch",
		TmuxName:  "outpost-demo-gen.batch",
		Command:   "python gen.py",
		StartedAt: time.Now().UTC(),
	}
	require.NoError(t, mirror.SaveSessionMeta(meta))

	loaded, err := mirror.LoadSessionMeta("personal", "demo", "gen.batch")
	require.NoError(t, err)
	require.Equal(t, meta.Name, loaded.Name)
	require.Equal(t, meta.Command, loaded.Command)
}

func TestLoadSessionMetaFallsBackToLegacyPath(t *testing.T) {
	home := t.TempDir()
	cfgDir := testenv.UseHomeConfigDir(t, home)
	sessionsDir := cfgDir + "/mirror-sessions"
	require.NoError(t, os.MkdirAll(sessionsDir, 0700))
	legacyPath := sessionsDir + "/personal_demo_gen-batch.json"
	require.NoError(t, os.WriteFile(legacyPath, []byte(`{
  "host": "personal",
  "project": "demo",
  "name": "gen.batch",
  "tmux_name": "outpost-demo-gen.batch",
  "command": "python gen.py",
  "started_at": "2026-01-02T03:04:05Z"
}`), 0600))

	loaded, err := mirror.LoadSessionMeta("personal", "demo", "gen.batch")
	require.NoError(t, err)
	require.Equal(t, "gen.batch", loaded.Name)
}

func TestSessionStatusIgnoresExitWhileRunning(t *testing.T) {
	exec := mock.New()
	exec.Responses["tmux has-session -t outpost-demo-gen 2>/dev/null"] = mockExit(0)
	exec.Responses[tailLogCmd("gen")] = mockStdout("stale\nEXIT:99\n")

	proj := &config.Project{Name: "demo", RemoteDir: demoRemoteDir}
	runner := mirror.New(exec, proj, t.TempDir(), "personal", nil)
	status, err := runner.SessionStatus(context.Background(), "gen")
	require.NoError(t, err)
	require.True(t, status.Running)
	require.Nil(t, status.ExitCode)
}

func TestPruneOldFinishedSessionMeta(t *testing.T) {
	home := t.TempDir()
	testenv.UseHomeConfigDir(t, home)

	require.NoError(t, mirror.SaveSessionMeta(mirror.SessionMeta{
		Host:      "personal",
		Project:   "demo",
		Name:      "old-batch",
		TmuxName:  "outpost-demo-old-batch",
		Command:   "python train.py",
		StartedAt: time.Now().UTC().Add(-8 * 24 * time.Hour),
	}))

	exec := mock.New()
	exec.Responses["command -v tmux >/dev/null 2>&1"] = mockOK()
	exec.Responses["tmux list-sessions -F '#{session_name}' 2>/dev/null || true"] = mockStdout("")

	proj := &config.Project{Name: "demo", RemoteDir: demoRemoteDir}
	runner := mirror.New(exec, proj, t.TempDir(), "personal", nil)
	sessions, err := runner.ListSessions(context.Background())
	require.NoError(t, err)
	require.Empty(t, sessions)

	_, err = mirror.LoadSessionMeta("personal", "demo", "old-batch")
	require.Error(t, err)
}

func TestParseExitLineViaStatus(t *testing.T) {
	exec := mock.New()
	logTail := strings.Join([]string{"line1", "EXIT:42"}, "\n")
	exec.Responses["tmux has-session -t outpost-demo-gen 2>/dev/null"] = mockExit(1)
	exec.Responses[tailLogCmd("gen")] = mockStdout(logTail)

	proj := &config.Project{Name: "demo", RemoteDir: demoRemoteDir}
	runner := mirror.New(exec, proj, t.TempDir(), "personal", nil)
	status, err := runner.SessionStatus(context.Background(), "gen")
	require.NoError(t, err)
	require.NotNil(t, status.ExitCode)
	require.Equal(t, 42, *status.ExitCode)
}

func tailLogCmd(shortName string) string {
	logPath := demoRemoteDir + "/.outpost/sessions/" + shortName + ".log"
	return "cd '" + demoRemoteDir + "' && tail -n 50 " + logPath + " 2>/dev/null || true"
}

func mockOK() struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
} {
	return struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{}
}

func mockStdout(stdout string) struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
} {
	return struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: stdout}
}

func mockExit(code int) struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
} {
	return struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{ExitCode: code}
}
