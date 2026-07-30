package mirror

import (
	"context"
	"fmt"
	"os"
	"strings"

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
	err = r.Exec.RunInteractive(ctx, cmd, opts)
	if exitErr, ok := err.(*transport.ExitError); ok {
		os.Exit(exitErr.Code)
	}
	return err
}
