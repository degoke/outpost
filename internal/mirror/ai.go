package mirror

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/degoke/outpost/internal/environment"
	"github.com/degoke/outpost/internal/transport"
	"github.com/degoke/outpost/internal/upload"
)

type AIOptions struct {
	Command string
	NoPull  bool
}

var defaultAIAgents = []string{"opencode", "claude", "codex"}

// AI syncs the repository, runs an interactive AI agent in the managed
// development environment, keeps local edits flowing up while the session is
// open, and pulls remote file changes back on exit. The returned exit code is
// the agent process exit code when the session ends normally.
func (r *Runner) AI(ctx context.Context, opts AIOptions) (int, error) {
	if !r.Proj.EnvironmentEnabled() {
		return 1, fmt.Errorf("outpost ai requires a managed development environment — set environment.enabled: true in project.yaml")
	}

	reason, err := r.syncIfNeeded(ctx, false)
	if err != nil {
		return 1, err
	}
	r.logSyncSkip(reason)

	agentCmd, err := r.resolveAICommand(ctx, opts.Command)
	if err != nil {
		return 1, err
	}

	stopWatch, watchDone := r.startBackgroundWatch(ctx)

	env := environment.New(r.Exec, r.Proj, r.Cwd)
	workdir := env.Workdir()
	inner := fmt.Sprintf("cd %s && if [ -f .venv/bin/activate ]; then . .venv/bin/activate; fi; %s",
		shellQuote(workdir), agentCmd)

	if r.Out != nil && !r.Out.JSON {
		r.Out.Step("Starting %s in remote environment — type exit or Ctrl-D to finish", strings.Fields(agentCmd)[0])
	}

	runErr := env.ExecInteractiveCommand(ctx, inner, transport.RunOpts{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	})

	stopWatch()
	watchErr := <-watchDone

	var pullErr error
	if !opts.NoPull {
		pullErr = r.PullRepo(ctx)
	}

	otherErr := firstError(watchErr, pullErr)
	if runErr != nil {
		if exitErr, ok := runErr.(*transport.ExitError); ok {
			return exitErr.Code, otherErr
		}
		return 1, firstError(runErr, otherErr)
	}
	return 0, otherErr
}

func (r *Runner) resolveAICommand(ctx context.Context, override string) (string, error) {
	override = strings.TrimSpace(override)
	if override != "" {
		return override, nil
	}
	if r.Proj.AI != nil {
		if cmd := strings.TrimSpace(r.Proj.AI.Command); cmd != "" {
			return cmd, nil
		}
	}
	env := environment.New(r.Exec, r.Proj, r.Cwd)
	for _, name := range defaultAIAgents {
		check := fmt.Sprintf("command -v %s >/dev/null 2>&1", name)
		code, err := env.ExecCommand(ctx, check, transport.RunOpts{})
		if err != nil {
			return "", err
		}
		if code == 0 {
			return name, nil
		}
	}
	return "", fmt.Errorf("no AI agent found in the remote environment — install opencode, claude, or codex, or set ai.command in project.yaml")
}

// ResolveAICommandForTest exposes agent resolution for tests.
func (r *Runner) ResolveAICommandForTest(ctx context.Context, override string) (string, error) {
	return r.resolveAICommand(ctx, override)
}

// PullRepo downloads project files from the remote host to the local repo and
// updates the local sync fingerprint so a follow-up push is skipped when
// nothing else changed.
func (r *Runner) PullRepo(ctx context.Context) error {
	if r.Out != nil && !r.Out.JSON {
		r.Out.Step("Syncing remote changes back to your machine...")
	}
	if err := upload.PullRepo(r.Cwd, r.Proj, r.Exec, r.syncOpts()); err != nil {
		return err
	}
	return upload.MarkRepoSynced(r.Cwd, r.Host, r.Proj.Name)
}

func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
