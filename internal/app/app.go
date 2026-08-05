package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/inspect"
	"github.com/degoke/outpost/internal/output"
	"github.com/degoke/outpost/internal/transport"
)

// Runner manages the Docker image and container produced from one project's Dockerfile.
type Runner struct {
	Exec    transport.Executor
	Project *config.Project
	Out     *output.Printer
}

func (r *Runner) Image() string     { return "outpost-app-" + config.SanitizeProjectName(r.Project.Name) }
func (r *Runner) Container() string { return r.Image() }

func (r *Runner) Build(ctx context.Context, dockerfile, buildContext string) error {
	dockerfile = strings.TrimSpace(dockerfile)
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}
	buildContext = strings.TrimSpace(buildContext)
	if buildContext == "" {
		buildContext = "."
	}
	cmd := fmt.Sprintf("cd %s && docker build -t %s -f %s %s", quote(r.Project.RemoteDir), quote(r.Image()), quote(dockerfile), quote(buildContext))
	code, err := r.Exec.Run(ctx, cmd, transport.RunOpts{Stdout: r.stdout(), Stderr: r.stderr()})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("application image build failed (exit %d)", code)
	}
	if r.Out != nil && !r.Out.JSON {
		r.Out.Success("Built application image %s", r.Image())
	}
	return nil
}

func (r *Runner) Run(ctx context.Context, ports []string, detach bool, args []string) (int, error) {
	parts := []string{"docker run", "--name", quote(r.Container())}
	for _, port := range ports {
		parts = append(parts, "-p", quote(port))
	}
	if detach {
		parts = append(parts, "-d")
	}
	parts = append(parts, quote(r.Image()))
	for _, arg := range args {
		parts = append(parts, quote(arg))
	}
	opts := transport.RunOpts{Stdin: os.Stdin, Stdout: r.stdout(), Stderr: r.stderr()}
	if detach {
		code, err := r.Exec.Run(ctx, strings.Join(parts, " "), opts)
		if err != nil {
			return code, err
		}
		if code == 0 && r.Out != nil && !r.Out.JSON {
			r.Out.Success("Started application container %s", r.Container())
		}
		return code, nil
	}
	return r.Exec.Run(ctx, strings.Join(parts, " "), opts)
}

func (r *Runner) Stop(ctx context.Context) error {
	code, err := r.Exec.Run(ctx, fmt.Sprintf("docker rm -f %s", quote(r.Container())), transport.RunOpts{Stdout: r.stdout(), Stderr: r.stderr()})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("application container stop failed (exit %d)", code)
	}
	return nil
}

func (r *Runner) Logs(ctx context.Context, follow bool) error {
	args := "docker logs"
	if follow {
		args += " -f"
	}
	code, err := r.Exec.Run(ctx, args+" "+quote(r.Container()), transport.RunOpts{Stdin: os.Stdin, Stdout: r.stdout(), Stderr: r.stderr()})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("application logs failed (exit %d)", code)
	}
	return nil
}

func (r *Runner) Status(ctx context.Context) (string, error) {
	// Docker returns exit 1 when the standalone application has not been
	// started. Make that normal state observable instead of leaking Docker's
	// template/inspect error to the user.
	return inspect.RunOutput(ctx, r.Exec, fmt.Sprintf("docker inspect --format '{{.State.Status}}' %s 2>/dev/null || printf 'missing\\n'", quote(r.Container())))
}

func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }

func (r *Runner) stdout() io.Writer {
	if r.Out == nil || r.Out.Stdout == nil {
		return io.Discard
	}
	return r.Out.Stdout
}

func (r *Runner) stderr() io.Writer {
	if r.Out == nil || r.Out.Stderr == nil {
		return io.Discard
	}
	return r.Out.Stderr
}
