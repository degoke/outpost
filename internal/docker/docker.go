package docker

import (
	"context"
	"os"
	"strings"

	"github.com/degoke/outpost/internal/transport"
)

func Run(ctx context.Context, exec transport.Executor, args []string) (int, error) {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = shellQuoteArg(arg)
	}
	cmd := "docker " + strings.Join(quoted, " ")
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

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
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

func wantsInteractive(args []string) bool {
	for _, a := range args {
		if a == "-it" || a == "-i" || a == "-t" || strings.HasPrefix(a, "-it") {
			return true
		}
	}
	return false
}
