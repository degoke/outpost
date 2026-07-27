package mirror

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/goke/outpost/internal/transport"
	"github.com/goke/outpost/internal/upload"
)

type RunOptions struct {
	Detach      bool
	SessionName string
	NoSync      bool
	NoVenv      bool
	Command     string
}

type RunResult struct {
	ExitCode    int
	SessionName string
}

func (r *Runner) Sync(ctx context.Context) error {
	return upload.SyncRepo(r.Cwd, r.Proj, r.Exec)
}

func (r *Runner) Run(ctx context.Context, opts RunOptions) (RunResult, error) {
	if !opts.NoSync {
		if err := r.Sync(ctx); err != nil {
			return RunResult{ExitCode: 1}, err
		}
	}

	cmd := opts.Command
	if strings.TrimSpace(cmd) == "" {
		return RunResult{ExitCode: 1}, fmt.Errorf("command is required")
	}

	venvExists, err := r.RemoteVenvPython(ctx)
	if err != nil {
		return RunResult{ExitCode: 1}, err
	}
	cmd = RewritePythonCommand(venvExists, r.VenvPath(), cmd, opts.NoVenv)

	if opts.Detach {
		return r.runDetached(ctx, opts, cmd)
	}
	code, err := r.runForeground(ctx, cmd)
	return RunResult{ExitCode: code}, err
}

func (r *Runner) runForeground(ctx context.Context, cmd string) (int, error) {
	runOpts := transport.RunOpts{
		WorkDir: r.Proj.RemoteDir,
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	}
	if isTerminal(os.Stdin) {
		err := r.Exec.RunInteractive(ctx, cmd, runOpts)
		if exitErr, ok := err.(*transport.ExitError); ok {
			return exitErr.Code, nil
		}
		return 0, err
	}
	return r.Exec.Run(ctx, cmd, runOpts)
}

func (r *Runner) runDetached(ctx context.Context, opts RunOptions, cmd string) (RunResult, error) {
	if err := EnsureTmux(ctx, r.Exec); err != nil {
		return RunResult{ExitCode: 1}, err
	}

	shortName := strings.TrimSpace(opts.SessionName)
	if shortName == "" {
		shortName = DefaultSessionName()
	}
	var err error
	shortName, err = SanitizeSessionName(shortName)
	if err != nil {
		return RunResult{ExitCode: 1}, err
	}

	tmuxName := TmuxSessionName(r.Proj, shortName)

	hasCmd := fmt.Sprintf("tmux has-session -t %s 2>/dev/null", shellQuote(tmuxName))
	if code, err := r.Exec.Run(ctx, hasCmd, transport.RunOpts{}); err != nil {
		return RunResult{ExitCode: 1}, err
	} else if code == 0 {
		return RunResult{ExitCode: 1}, fmt.Errorf("session %q already exists — choose another name or run 'outpost mirror sessions kill %s'", shortName, shortName)
	}

	sessionsDir := remoteSessionsDir(r.Proj)
	logPath := remoteSessionLog(r.Proj, shortName)

	mkdirCmd := fmt.Sprintf("mkdir -p %s", shellQuote(sessionsDir))
	if code, err := r.Exec.Run(ctx, mkdirCmd, transport.RunOpts{WorkDir: r.Proj.RemoteDir}); err != nil {
		return RunResult{ExitCode: 1}, err
	} else if code != 0 {
		return RunResult{ExitCode: 1}, fmt.Errorf("could not create remote session log directory")
	}

	inner := detachedInnerCommand(cmd, logPath)
	bashCmd := "bash -lc " + shellQuote(inner)
	tmuxCmd := fmt.Sprintf("tmux new-session -d -s %s -c %s %s",
		shellQuote(tmuxName),
		shellQuote(r.Proj.RemoteDir),
		shellQuote(bashCmd),
	)

	code, err := r.Exec.Run(ctx, tmuxCmd, transport.RunOpts{})
	if err != nil {
		return RunResult{ExitCode: 1}, err
	}
	if code != 0 {
		return RunResult{ExitCode: 1}, fmt.Errorf("failed to start detached session %q", shortName)
	}

	meta := SessionMeta{
		Host:      r.Host,
		Project:   r.Proj.Name,
		Name:      shortName,
		TmuxName:  tmuxName,
		Command:   opts.Command,
		StartedAt: nowUTC(),
	}
	if err := SaveSessionMeta(meta); err != nil {
		return RunResult{ExitCode: 1}, err
	}
	return RunResult{SessionName: shortName}, nil
}

func isTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func detachedInnerCommand(cmd, logPath string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(cmd))
	return fmt.Sprintf(
		`eval "$(printf %%s %s | base64 -d)" 2>&1 | tee -a %s; echo EXIT:$? >> %s`,
		shellQuote(encoded),
		shellQuote(logPath),
		shellQuote(logPath),
	)
}
