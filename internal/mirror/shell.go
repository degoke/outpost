package mirror

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/degoke/outpost/internal/environment"
	"github.com/degoke/outpost/internal/transport"
)

func (r *Runner) Shell(ctx context.Context) error {
	if r.Proj.EnvironmentEnabled() {
		if _, err := r.syncIfNeeded(ctx, false); err != nil {
			return err
		}
		stopWatch, watchDone := r.startBackgroundWatch(ctx)
		err := environment.New(r.Exec, r.Proj, r.Cwd).Shell(ctx, transport.RunOpts{
			Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr,
		})
		stopWatch()
		if watchErr := <-watchDone; err == nil && watchErr != nil {
			return watchErr
		}
		return err
	}
	if err := transport.EnsureRemoteDir(r.Exec, r.Proj.RemoteDir); err != nil {
		return err
	}
	venvExists, err := r.RemoteVenvPython(ctx)
	if err != nil {
		return err
	}

	pathExport := ""
	if r.Proj.ToolchainAuto() {
		plan, err := DetectPlan(r.Cwd, r.Proj, "")
		if err != nil {
			return err
		}
		if !plan.Empty() {
			pathPrefixes, err := r.ensureToolchainWithCache(ctx, plan, r.Out)
			if err != nil {
				return err
			}
			if len(pathPrefixes) > 0 {
				pathExport = "export PATH=" + strings.Join(pathPrefixes, ":") + ":$PATH && "
			}
		}
	}

	var cmd string
	if venvExists {
		activate := r.VenvPath() + "/bin/activate"
		cmd = fmt.Sprintf("bash -lc %s",
			shellQuote(fmt.Sprintf("%scd %s && source %s && exec bash", pathExport, r.Proj.RemoteDir, activate)),
		)
	} else if pathExport != "" {
		cmd = fmt.Sprintf("bash -lc %s",
			shellQuote(fmt.Sprintf("%scd %s && exec bash", pathExport, r.Proj.RemoteDir)),
		)
	} else {
		cmd = "bash"
	}

	opts := transport.RunOpts{
		WorkDir: r.Proj.RemoteDir,
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	}
	stopWatch, watchDone := r.startBackgroundWatch(ctx)
	err = r.Exec.RunInteractive(ctx, cmd, opts)
	stopWatch()
	watchErr := <-watchDone
	if err == nil && watchErr != nil {
		return watchErr
	}
	return err
}

// startBackgroundWatch keeps the remote workspace synchronized while an
// interactive project shell is open. The initial sync has already happened
// before the shell starts, so the watcher only handles subsequent changes.
func (r *Runner) startBackgroundWatch(ctx context.Context) (func(), <-chan error) {
	watchCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- r.Watch(watchCtx, WatchOptions{SkipInitialSync: true})
	}()
	return func() { cancel() }, done
}
