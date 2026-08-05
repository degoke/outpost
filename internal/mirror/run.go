package mirror

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/degoke/outpost/internal/environment"
	"github.com/degoke/outpost/internal/transport"
)

type RunOptions struct {
	Detach         bool
	Foreground     bool
	AttachSession  string
	SessionName    string
	NoSync         bool
	ForceSync      bool
	NoVenv         bool
	NoToolchain    bool
	Command        string
}

type RunResult struct {
	ExitCode    int    `json:"exit_code"`
	SessionName string `json:"session_name,omitempty"`
	Detached    bool   `json:"detached,omitempty"`
}

// ExitSessionDetached is returned when the user detaches from a tmux session
// that is still running. Scripts can distinguish this from a completed run.
const ExitSessionDetached = 130

func (r *Runner) Sync(ctx context.Context) error {
	return r.SyncWith(ctx, SyncOptions{
		UseRsync: r.SyncUseRsync,
		Workers:  r.SyncWorkers,
	})
}

func (r *Runner) Run(ctx context.Context, opts RunOptions) (RunResult, error) {
	if opts.AttachSession != "" {
		return r.attachAndFinish(ctx, opts.AttachSession)
	}

	if !opts.NoSync {
		if opts.ForceSync {
			if err := r.syncAndRecord(ctx); err != nil {
				return RunResult{ExitCode: 1}, err
			}
		} else {
			reason, err := r.syncIfNeeded(ctx, false)
			if err != nil {
				return RunResult{ExitCode: 1}, err
			}
			if reason != SyncSkippedNone {
				r.logSyncSkip(reason)
			}
		}
	}

	cmd := opts.Command
	if strings.TrimSpace(cmd) == "" {
		return RunResult{ExitCode: 1}, fmt.Errorf("command is required")
	}
	cmd = r.withProjectKubeconfig(cmd)

	var err error
	if !r.Proj.EnvironmentEnabled() {
		cmd, err = r.ensureToolchainForRun(ctx, cmd, opts.NoToolchain)
		if err != nil {
			return RunResult{ExitCode: 1}, err
		}
		venvExists, err := r.RemoteVenvPython(ctx)
		if err != nil {
			return RunResult{ExitCode: 1}, err
		}
		cmd = RewritePythonCommand(venvExists, r.VenvPath(), cmd, opts.NoVenv)
	}

	if opts.Detach || (IsInteractiveTerminal() && !opts.Foreground) {
		result, err := r.runDetached(ctx, opts, cmd)
		if err != nil || opts.Detach {
			return result, err
		}
		return r.attachAndFinish(ctx, result.SessionName)
	}
	code, err := r.runForeground(ctx, cmd)
	return RunResult{ExitCode: code}, err
}

func (r *Runner) attachAndFinish(ctx context.Context, sessionName string) (RunResult, error) {
	if err := r.AttachSession(ctx, sessionName); err != nil {
		if finished, ok := IsSessionFinished(err); ok {
			return RunResult{ExitCode: finished.ExitCode, SessionName: finished.Name}, nil
		}
		return RunResult{ExitCode: 1, SessionName: sessionName}, err
	}
	status, err := r.SessionStatus(ctx, sessionName)
	if err != nil {
		return RunResult{ExitCode: 1, SessionName: sessionName}, err
	}
	result := RunResult{SessionName: sessionName}
	if status.Running {
		if r.Out != nil && !r.Out.JSON {
			r.Out.Info("Session %q is still running", sessionName)
			r.Out.Info("Reconnect with: outpost session attach %s", sessionName)
		}
		result.ExitCode = ExitSessionDetached
		result.Detached = true
		return result, nil
	}
	if status.ExitCode != nil {
		result.ExitCode = *status.ExitCode
		return result, nil
	}
	if r.Out != nil && !r.Out.JSON {
		r.Out.Info("Session %q finished but exit code is unknown", sessionName)
	}
	result.ExitCode = 1
	return result, nil
}

// withProjectKubeconfig makes kubectl commands use the project cluster's
// persisted config. The config is mounted with the project directory inside
// the managed container; relying on kind's default ~/.kube/config leaves its
// API server set to 0.0.0.0, which is not reachable from the container.
func (r *Runner) withProjectKubeconfig(cmd string) string {
	if r.Proj == nil || r.Proj.Kubernetes == nil || strings.Contains(cmd, "KUBECONFIG=") {
		return cmd
	}
	trimmed := strings.TrimSpace(cmd)
	if trimmed != "kubectl" && !strings.HasPrefix(trimmed, "kubectl ") {
		return cmd
	}
	workdir := "/workspace"
	if r.Proj.Environment != nil && strings.TrimSpace(r.Proj.Environment.Workdir) != "" {
		workdir = strings.TrimSpace(r.Proj.Environment.Workdir)
	}
	return "KUBECONFIG=" + shellQuote(filepath.Join(workdir, ".outpost", "kubernetes", "kubeconfig")) + " " + cmd
}

func (r *Runner) runForeground(ctx context.Context, cmd string) (int, error) {
	if r.Proj.EnvironmentEnabled() {
		return environment.New(r.Exec, r.Proj, r.Cwd).ExecCommand(ctx, cmd, transport.RunOpts{
			Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr,
		})
	}
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
	if r.Proj.EnvironmentEnabled() {
		if err := environment.New(r.Exec, r.Proj, r.Cwd).Ensure(ctx); err != nil {
			return RunResult{ExitCode: 1}, err
		}
		container := environment.New(r.Exec, r.Proj, r.Cwd).Name()
		cmd = fmt.Sprintf("docker exec %s %s -lc %s", shellQuote(container), shellQuote("/bin/bash"), shellQuote(cmd))
	}
	if err := EnsureTmux(ctx, r.Exec, r.Out); err != nil {
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
		return RunResult{ExitCode: 1}, fmt.Errorf("session %q already exists — choose another name or clean up the existing session", shortName)
	}

	sessionsDir := remoteSessionsDir(r.Proj)
	logPath := remoteSessionLog(r.Proj, shortName)

	mkdirCmd := fmt.Sprintf("mkdir -p %s", shellQuote(sessionsDir))
	if code, err := r.Exec.Run(ctx, mkdirCmd, transport.RunOpts{WorkDir: r.Proj.RemoteDir}); err != nil {
		return RunResult{ExitCode: 1}, err
	} else if code != 0 {
		return RunResult{ExitCode: 1}, fmt.Errorf("could not create remote session log directory")
	}

	truncateCmd := fmt.Sprintf("> %s", shellQuote(logPath))
	if code, err := r.Exec.Run(ctx, truncateCmd, transport.RunOpts{WorkDir: r.Proj.RemoteDir}); err != nil {
		return RunResult{ExitCode: 1}, err
	} else if code != 0 {
		return RunResult{ExitCode: 1}, fmt.Errorf("could not prepare session log file")
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
		if r.Out != nil && !r.Out.JSON {
			r.Out.Info("Warning: started session %q but could not save local metadata: %v", shortName, err)
			r.Out.Info("Reconnect with: outpost session attach %s", shortName)
			r.Out.Info("Check status with: outpost session status %s", shortName)
		}
		return RunResult{SessionName: shortName}, nil
	}
	if r.Out != nil && !r.Out.JSON {
		r.Out.Success("Started run session %q", shortName)
		r.Out.Info("Reconnect with: outpost session attach %s", shortName)
		r.Out.Info("Check status with: outpost session status %s", shortName)
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

func IsInteractiveTerminal() bool {
	return isTerminal(os.Stdin) && isTerminal(os.Stdout)
}

func detachedInnerCommand(cmd, logPath string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(cmd))
	return fmt.Sprintf(
		`: 'EXIT:$?'; eval "$(printf %%s %s | base64 -d)" 2>&1 | tee -a %s; code=${PIPESTATUS[0]}; if [ -f %s ] && [ "$(wc -c < %s)" -gt 10485760 ]; then tail -c 10485760 %s > %s.tmp && mv %s.tmp %s; fi; echo EXIT:$code >> %s`,
		shellQuote(encoded),
		shellQuote(logPath),
		shellQuote(logPath), shellQuote(logPath), shellQuote(logPath), shellQuote(logPath), shellQuote(logPath), shellQuote(logPath),
		shellQuote(logPath),
	)
}
