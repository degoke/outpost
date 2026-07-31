package mirror

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/degoke/outpost/internal/environment"
	"github.com/degoke/outpost/internal/transport"
	"github.com/degoke/outpost/internal/upload"
)

type SetupPythonOptions struct {
	Python       string
	Requirements string
}

func (r *Runner) VenvPath() string {
	if r.Proj.Python != nil && strings.TrimSpace(r.Proj.Python.Venv) != "" {
		return strings.TrimSpace(r.Proj.Python.Venv)
	}
	return ".venv"
}

func (r *Runner) RequirementsPath() string {
	if r.Proj.Python != nil && strings.TrimSpace(r.Proj.Python.Requirements) != "" {
		return strings.TrimSpace(r.Proj.Python.Requirements)
	}
	return "requirements.txt"
}

func (r *Runner) RemoteVenvPython(ctx context.Context) (bool, error) {
	venv := r.VenvPath()
	cmd := fmt.Sprintf("test -x %s", shellQuote(venv+"/bin/python"))
	code, err := r.Exec.Run(ctx, cmd, transport.RunOpts{WorkDir: r.Proj.RemoteDir})
	if err != nil {
		return false, err
	}
	return code == 0, nil
}

func RewritePythonCommand(remoteVenvExists bool, venvPath, cmd string, noVenv bool) string {
	if noVenv || !remoteVenvExists {
		return cmd
	}
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return cmd
	}
	first := fields[0]
	if first == venvPath+"/bin/python" || strings.HasPrefix(first, venvPath+"/") {
		return cmd
	}
	if first != "python" && first != "python3" {
		return cmd
	}
	fields[0] = venvPath + "/bin/python"
	return strings.Join(fields, " ")
}

func (r *Runner) SetupPython(ctx context.Context, opts SetupPythonOptions) error {
	if r.Out != nil {
		r.Out.Step("Syncing repository...")
	}
	reason, err := r.syncIfNeeded(ctx, false)
	if err != nil {
		return err
	}
	if reason != SyncSkippedNone {
		r.logSyncSkip(reason)
	}
	python := strings.TrimSpace(opts.Python)
	if python == "" {
		python = "python3"
	}
	requirements := strings.TrimSpace(opts.Requirements)
	if requirements == "" {
		requirements = r.RequirementsPath()
	}
	venv := r.VenvPath()
	if r.Proj.EnvironmentEnabled() {
		manager := environment.New(r.Exec, r.Proj, r.Cwd)
		if err := manager.Ensure(ctx); err != nil {
			return err
		}
		check := fmt.Sprintf("test -x %s/bin/python", shellQuote(venv))
		code, err := manager.ExecCommand(ctx, check, transport.RunOpts{})
		if err != nil {
			return err
		}
		if code != 0 {
			code, err = manager.ExecCommand(ctx, fmt.Sprintf("%s -m venv %s", shellQuote(python), shellQuote(venv)), transport.RunOpts{})
			if err != nil {
				return err
			}
			if code != 0 {
				return fmt.Errorf("failed to create Python environment inside the development container")
			}
		}
		if _, err := os.Stat(filepath.Join(r.Cwd, requirements)); err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		code, err = manager.ExecCommand(ctx, fmt.Sprintf("%s/bin/pip install -r %s", shellQuote(venv), shellQuote(requirements)), transport.RunOpts{Stdout: os.Stdout, Stderr: os.Stderr})
		if err != nil {
			return err
		}
		if code != 0 {
			return fmt.Errorf("pip install failed inside the development container (exit %d)", code)
		}
		return nil
	}

	exists, err := r.RemoteVenvPython(ctx)
	if err != nil {
		return err
	}
	if !exists {
		if r.Out != nil {
			r.Out.Step("Creating Python virtual environment...")
		}
		createCmd := fmt.Sprintf("%s -m venv %s", shellQuote(python), shellQuote(venv))
		code, err := r.Exec.Run(ctx, createCmd, transport.RunOpts{WorkDir: r.Proj.RemoteDir})
		if err != nil {
			return err
		}
		if code != 0 {
			return fmt.Errorf("failed to create virtual environment with %s", python)
		}
	}

	reqLocal := filepath.Join(r.Cwd, requirements)
	if _, err := os.Stat(reqLocal); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	reqRemote := r.Proj.RemoteDir + "/" + filepath.ToSlash(requirements)
	needInstall, err := upload.NeedsUpload(r.Exec, reqLocal, reqRemote)
	if err != nil {
		return err
	}
	if !needInstall && exists {
		return nil
	}
	if r.Out != nil {
		r.Out.Step("Installing Python requirements from %s...", requirements)
	}
	installCmd := fmt.Sprintf("%s/bin/pip install -r %s",
		shellQuote(venv),
		shellQuote(requirements),
	)
	code, err := r.Exec.Run(ctx, installCmd, transport.RunOpts{WorkDir: r.Proj.RemoteDir})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("pip install failed (exit %d)", code)
	}
	return nil
}
