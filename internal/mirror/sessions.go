package mirror

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/transport"
)

type SessionMeta struct {
	Host      string    `json:"host"`
	Project   string    `json:"project"`
	Name      string    `json:"name"`
	TmuxName  string    `json:"tmux_name"`
	Command   string    `json:"command"`
	StartedAt time.Time `json:"started_at"`
}

type SessionStatus struct {
	Name      string
	Running   bool
	ExitCode  *int
	LogTail   string
	Command   string
	StartedAt time.Time
}

func MirrorSessionsDir() (string, error) {
	dir, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "mirror-sessions"), nil
}

func sessionMetaPath(host, project, name string) (string, error) {
	dir, err := MirrorSessionsDir()
	if err != nil {
		return "", err
	}
	safe := config.SanitizeProjectName(name)
	return filepath.Join(dir, fmt.Sprintf("%s_%s_%s.json", host, project, safe)), nil
}

func SaveSessionMeta(meta SessionMeta) error {
	dir, err := MirrorSessionsDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	path, err := sessionMetaPath(meta.Host, meta.Project, meta.Name)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func LoadSessionMeta(host, project, name string) (*SessionMeta, error) {
	path, err := sessionMetaPath(host, project, name)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var meta SessionMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func (r *Runner) ListSessions(ctx context.Context) ([]SessionStatus, error) {
	if err := EnsureTmux(ctx, r.Exec); err != nil {
		return nil, err
	}
	prefix := sessionPrefix(r.Proj)
	cmd := fmt.Sprintf("tmux list-sessions -F '#{session_name}' 2>/dev/null || true")
	var stdout strings.Builder
	code, err := r.Exec.Run(ctx, cmd, transport.RunOpts{Stdout: &stdout})
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("tmux list-sessions failed (exit %d)", code)
	}

	var statuses []SessionStatus
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, prefix) {
			continue
		}
		short, ok := ShortSessionName(r.Proj, line)
		if !ok {
			continue
		}
		status, err := r.sessionStatus(ctx, short, line)
		if err != nil {
			return nil, err
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

func (r *Runner) SessionStatus(ctx context.Context, shortName string) (SessionStatus, error) {
	shortName, err := SanitizeSessionName(shortName)
	if err != nil {
		return SessionStatus{}, err
	}
	tmuxName := TmuxSessionName(r.Proj, shortName)
	return r.sessionStatus(ctx, shortName, tmuxName)
}

func (r *Runner) sessionStatus(ctx context.Context, shortName, tmuxName string) (SessionStatus, error) {
	status := SessionStatus{Name: shortName}
	if meta, err := LoadSessionMeta(r.Host, r.Proj.Name, shortName); err == nil {
		status.Command = meta.Command
		status.StartedAt = meta.StartedAt
	}

	hasCmd := fmt.Sprintf("tmux has-session -t %s 2>/dev/null", shellQuote(tmuxName))
	code, err := r.Exec.Run(ctx, hasCmd, transport.RunOpts{})
	if err != nil {
		return status, err
	}
	status.Running = code == 0

	logPath := remoteSessionLog(r.Proj, shortName)
	tailCmd := fmt.Sprintf("tail -n 50 %s 2>/dev/null || true", shellQuote(logPath))
	var stdout strings.Builder
	_, err = r.Exec.Run(ctx, tailCmd, transport.RunOpts{WorkDir: r.Proj.RemoteDir, Stdout: &stdout})
	if err != nil {
		return status, err
	}
	status.LogTail = strings.TrimSpace(stdout.String())
	if exitCode := parseExitLine(status.LogTail); exitCode != nil {
		status.ExitCode = exitCode
	}
	return status, nil
}

func parseExitLine(log string) *int {
	lines := strings.Split(log, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "EXIT:") {
			continue
		}
		val := strings.TrimPrefix(line, "EXIT:")
		var code int
		if _, err := fmt.Sscanf(val, "%d", &code); err != nil {
			continue
		}
		return &code
	}
	return nil
}

func (r *Runner) AttachSession(ctx context.Context, shortName string) error {
	if err := EnsureTmux(ctx, r.Exec); err != nil {
		return err
	}
	shortName, err := SanitizeSessionName(shortName)
	if err != nil {
		return err
	}
	tmuxName := TmuxSessionName(r.Proj, shortName)
	hasCmd := fmt.Sprintf("tmux has-session -t %s 2>/dev/null", shellQuote(tmuxName))
	code, err := r.Exec.Run(ctx, hasCmd, transport.RunOpts{})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("session %q not found", shortName)
	}
	cmd := fmt.Sprintf("tmux attach -t %s", shellQuote(tmuxName))
	opts := transport.RunOpts{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	err = r.Exec.RunInteractive(ctx, cmd, opts)
	if exitErr, ok := err.(*transport.ExitError); ok {
		os.Exit(exitErr.Code)
	}
	return err
}

func (r *Runner) KillSession(ctx context.Context, shortName string) error {
	if err := EnsureTmux(ctx, r.Exec); err != nil {
		return err
	}
	shortName, err := SanitizeSessionName(shortName)
	if err != nil {
		return err
	}
	tmuxName := TmuxSessionName(r.Proj, shortName)
	cmd := fmt.Sprintf("tmux kill-session -t %s", shellQuote(tmuxName))
	code, err := r.Exec.Run(ctx, cmd, transport.RunOpts{})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("session %q not found", shortName)
	}
	path, err := sessionMetaPath(r.Host, r.Proj.Name, shortName)
	if err == nil {
		_ = os.Remove(path)
	}
	return nil
}

func nowUTC() time.Time {
	return time.Now().UTC()
}
