package mirror

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goke/outpost/internal/transport"
	"github.com/goke/outpost/internal/upload"
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
	if err := upload.SyncRepo(r.Cwd, r.Proj, r.Exec); err != nil {
		return err
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

	exists, err := r.RemoteVenvPython(ctx)
	if err != nil {
		return err
	}
	if !exists {
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
