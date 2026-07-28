package mirror

import (
	"context"
	"fmt"
	"os"

	"github.com/degoke/outpost/internal/transport"
)

func (r *Runner) Shell(ctx context.Context) error {
	if err := transport.EnsureRemoteDir(r.Exec, r.Proj.RemoteDir); err != nil {
		return err
	}
	venvExists, err := r.RemoteVenvPython(ctx)
	if err != nil {
		return err
	}

	var cmd string
	if venvExists {
		activate := r.VenvPath() + "/bin/activate"
		cmd = fmt.Sprintf("bash -lc %s",
			shellQuote(fmt.Sprintf("cd %s && source %s && exec bash", r.Proj.RemoteDir, activate)),
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
	err = r.Exec.RunInteractive(ctx, cmd, opts)
	if exitErr, ok := err.(*transport.ExitError); ok {
		os.Exit(exitErr.Code)
	}
	return err
}
