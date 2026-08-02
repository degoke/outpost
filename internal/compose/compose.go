package compose

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/environment"
	"github.com/degoke/outpost/internal/output"
	"github.com/degoke/outpost/internal/transport"
	"github.com/degoke/outpost/internal/upload"
)

type Runner struct {
	Exec     transport.Executor
	Project  *config.Project
	Cwd      string
	HostName string
	ForceYes bool
	Out      *output.Printer
}

func (r *Runner) BuildCommand(subcommand string, args []string) string {
	return r.buildCmd(subcommand, args)
}

func (r *Runner) buildCmd(subcommand string, args []string) string {
	base := r.Project.RemoteDir
	files := ""
	for _, f := range upload.AllComposeFiles(r.Project) {
		files += " -f " + shellQuoteArg(base+"/"+filepath.Base(f))
	}
	return fmt.Sprintf("docker compose -p %s %s %s %s",
		shellQuote(r.Project.Name),
		files,
		subcommand,
		strings.Join(shellQuoteArgs(args), " "),
	)
}

func shellQuoteArgs(args []string) []string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = shellQuoteArg(arg)
	}
	return quoted
}

func shellQuoteArg(s string) string {
	if s != "" {
		for _, r := range s {
			if !(r == '-' || r == '_' || r == '.' || r == '/' || r == ':' || r == '=' || r == ',' || r == '@' ||
				(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
				return shellQuote(s)
			}
		}
		return s
	}
	return shellQuote(s)
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func (r *Runner) Run(ctx context.Context, subcommand string, args []string, uploadFirst bool) (int, error) {
	if err := r.Project.RequireCompose(); err != nil {
		return 1, err
	}
	if uploadFirst {
		if err := r.syncProjectIfNeeded(); err != nil {
			return 1, err
		}
	}
	if r.Project.EnvironmentEnabled() && subcommand != "down" {
		if err := environment.New(r.Exec, r.Project, r.Cwd).Ensure(ctx); err != nil {
			return 1, err
		}
	}
	if subcommand == "up" {
		if err := EnsureImported(ctx, r.Exec, r.Cwd, r.Project, r.HostName, r.ForceYes); err != nil {
			return 1, err
		}
		if err := CheckComposeCapacity(ctx, r.Exec, r.Cwd, r.Project); err != nil {
			return 1, err
		}
	}
	if wantsInteractive(subcommand, args) {
		args = transport.StripChildTTY(args)
	}
	cmd := r.buildCmd(subcommand, args)
	opts := transport.RunOpts{
		WorkDir: r.Project.RemoteDir,
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	}
	if wantsInteractive(subcommand, args) {
		err := r.Exec.RunInteractive(ctx, cmd, opts)
		if exitErr, ok := err.(*transport.ExitError); ok {
			return exitErr.Code, nil
		}
		return 0, err
	}
	return r.Exec.Run(ctx, cmd, opts)
}

func wantsInteractive(subcommand string, args []string) bool {
	if subcommand == "logs" {
		for _, a := range args {
			if a == "-f" || a == "--follow" {
				return true
			}
		}
	}
	if subcommand == "exec" {
		for _, a := range args {
			if a == "-it" || a == "-i" || a == "-t" || strings.HasPrefix(a, "-it") {
				return true
			}
		}
	}
	return false
}

func IsDestructive(subcommand string, args []string) bool {
	if subcommand != "down" {
		return false
	}
	for _, a := range args {
		if a == "-v" || a == "--volumes" {
			return true
		}
	}
	return true // down is always somewhat destructive
}
