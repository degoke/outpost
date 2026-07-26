package docker

import (
	"context"
	"os"
	"strings"

	"github.com/goke/outpost/internal/transport"
)

func Run(ctx context.Context, exec transport.Executor, args []string) (int, error) {
	cmd := "docker " + strings.Join(args, " ")
	opts := transport.RunOpts{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	if wantsInteractive(args) {
		err := exec.RunInteractive(ctx, cmd, opts)
		if exitErr, ok := err.(*transport.ExitError); ok {
			return exitErr.Code, nil
		}
		return 0, err
	}
	return exec.Run(ctx, cmd, opts)
}

func wantsInteractive(args []string) bool {
	for _, a := range args {
		if a == "-it" || a == "-i" || a == "-t" || strings.HasPrefix(a, "-it") {
			return true
		}
	}
	return false
}
