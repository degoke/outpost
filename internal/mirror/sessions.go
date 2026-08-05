package mirror

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/transport"
)

const finishedSessionMetaRetention = 7 * 24 * time.Hour

// SessionFinishedError is returned when attaching to a session that already exited.
type SessionFinishedError struct {
	Name     string
	ExitCode int
}

func (e *SessionFinishedError) Error() string {
	return fmt.Sprintf("session %q finished with exit code %d", e.Name, e.ExitCode)
}

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
	safe, err := SanitizeSessionName(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fmt.Sprintf("%s_%s_%s.json", host, project, safe)), nil
}

func sessionMetaLegacyPath(host, project, name string) (string, error) {
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
	if err != nil && os.IsNotExist(err) {
		legacyPath, legErr := sessionMetaLegacyPath(host, project, name)
		if legErr != nil {
			return nil, err
		}
		data, err = os.ReadFile(legacyPath)
	}
	if err != nil {
		return nil, err
	}
	var meta SessionMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func ListSessionMeta(host, project string) ([]SessionMeta, error) {
	dir, err := MirrorSessionsDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	prefix := host + "_" + project + "_"
	var metas []SessionMeta
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".json") {
			continue
		}
		if !strings.HasPrefix(ent.Name(), prefix) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, ent.Name()))
		if err != nil {
			continue
		}
		var meta SessionMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}
		if meta.Host != host || meta.Project != project {
			continue
		}
		metas = append(metas, meta)
	}
	return metas, nil
}

func DeleteSessionMeta(host, project, name string) error {
	path, err := sessionMetaPath(host, project, name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	legacyPath, legErr := sessionMetaLegacyPath(host, project, name)
	if legErr != nil {
		return nil
	}
	if err := os.Remove(legacyPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func pruneFinishedSessionMeta(host, project string, status SessionStatus) {
	if finishedSessionMetaRetention <= 0 {
		return
	}
	if status.Running || status.ExitCode == nil || status.StartedAt.IsZero() {
		return
	}
	if time.Since(status.StartedAt) <= finishedSessionMetaRetention {
		return
	}
	_ = DeleteSessionMeta(host, project, status.Name)
}

func (r *Runner) ListSessions(ctx context.Context) ([]SessionStatus, error) {
	if err := EnsureTmux(ctx, r.Exec, r.Out); err != nil {
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

	seen := make(map[string]struct{})
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
		pruneFinishedSessionMeta(r.Host, r.Proj.Name, status)
		statuses = append(statuses, status)
		seen[short] = struct{}{}
	}

	metas, err := ListSessionMeta(r.Host, r.Proj.Name)
	if err != nil {
		return nil, err
	}
	for _, meta := range metas {
		if _, ok := seen[meta.Name]; ok {
			continue
		}
		if finishedSessionMetaRetention > 0 && !meta.StartedAt.IsZero() && time.Since(meta.StartedAt) > finishedSessionMetaRetention {
			_ = DeleteSessionMeta(meta.Host, meta.Project, meta.Name)
			continue
		}
		status, err := r.sessionStatus(ctx, meta.Name, meta.TmuxName)
		if err != nil {
			return nil, err
		}
		if status.Command == "" {
			status.Command = meta.Command
		}
		if status.StartedAt.IsZero() {
			status.StartedAt = meta.StartedAt
		}
		pruneFinishedSessionMeta(r.Host, r.Proj.Name, status)
		if !status.Running && status.ExitCode != nil && finishedSessionMetaRetention > 0 && !status.StartedAt.IsZero() && time.Since(status.StartedAt) > finishedSessionMetaRetention {
			continue
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
	if !status.Running {
		if exitCode := parseExitLine(status.LogTail); exitCode != nil {
			status.ExitCode = exitCode
		}
	}
	return status, nil
}

// SessionLogs prints the session log from the remote host.
func (r *Runner) SessionLogs(ctx context.Context, shortName string, follow bool, lines int) error {
	shortName, err := SanitizeSessionName(shortName)
	if err != nil {
		return err
	}
	if lines <= 0 {
		lines = 50
	}
	logPath := remoteSessionLog(r.Proj, shortName)
	if follow {
		cmd := fmt.Sprintf("tail -n %d -f %s", lines, shellQuote(logPath))
		return r.Exec.RunInteractive(ctx, cmd, transport.RunOpts{
			Stdout: os.Stdout,
			Stderr: os.Stderr,
		})
	}
	cmd := fmt.Sprintf("tail -n %d %s 2>/dev/null || true", lines, shellQuote(logPath))
	var stdout strings.Builder
	code, err := r.Exec.Run(ctx, cmd, transport.RunOpts{WorkDir: r.Proj.RemoteDir, Stdout: &stdout})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("read session log failed (exit %d)", code)
	}
	if out := strings.TrimSpace(stdout.String()); out != "" {
		fmt.Fprintln(os.Stdout, out)
	}
	return nil
}

func IsSessionFinished(err error) (*SessionFinishedError, bool) {
	var finished *SessionFinishedError
	if errors.As(err, &finished) {
		return finished, true
	}
	return nil, false
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
	if err := EnsureTmux(ctx, r.Exec, r.Out); err != nil {
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
		status, statErr := r.sessionStatus(ctx, shortName, tmuxName)
		if statErr == nil && status.ExitCode != nil {
			if status.LogTail != "" {
				_, _ = fmt.Fprintf(os.Stdout, "%s\n", status.LogTail)
			}
			return &SessionFinishedError{Name: shortName, ExitCode: *status.ExitCode}
		}
		return fmt.Errorf("session %q not found", shortName)
	}
	cmd := fmt.Sprintf("tmux attach -t %s", shellQuote(tmuxName))
	opts := transport.RunOpts{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	err = r.Exec.RunInteractive(ctx, cmd, opts)
	return err
}

func (r *Runner) KillSession(ctx context.Context, shortName string) error {
	if err := EnsureTmux(ctx, r.Exec, r.Out); err != nil {
		return err
	}
	shortName, err := SanitizeSessionName(shortName)
	if err != nil {
		return err
	}
	tmuxName := TmuxSessionName(r.Proj, shortName)
	if r.Out != nil {
		r.Out.Step("Stopping session %q...", shortName)
	}
	cmd := fmt.Sprintf("tmux kill-session -t %s", shellQuote(tmuxName))
	code, err := r.Exec.Run(ctx, cmd, transport.RunOpts{})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("session %q not found", shortName)
	}
	_, _ = r.Exec.Run(ctx, fmt.Sprintf("rm -f %s", shellQuote(remoteSessionLog(r.Proj, shortName))), transport.RunOpts{})
	return DeleteSessionMeta(r.Host, r.Proj.Name, shortName)
}

func nowUTC() time.Time {
	return time.Now().UTC()
}
