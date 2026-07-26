package machine

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/goke/outpost/internal/transport"
)

func (s *Service) Shell(ctx context.Context, name string) error {
	incusName, err := s.resolveIncusName(ctx, name)
	if err != nil {
		return err
	}
	shell := "bash"
	checkCmd := fmt.Sprintf("incus exec %s -- test -x /bin/bash", shellQuote(incusName))
	code, _ := s.Exec.Run(ctx, checkCmd, transport.RunOpts{})
	if code != 0 {
		shell = "sh"
	}
	cmd := fmt.Sprintf("incus exec -t %s -- %s", shellQuote(incusName), shell)
	opts := transport.RunOpts{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	err = s.Exec.RunInteractive(ctx, cmd, opts)
	if exitErr, ok := err.(*transport.ExitError); ok {
		os.Exit(exitErr.Code)
	}
	return err
}

func (s *Service) RunCommand(ctx context.Context, name string, args []string) (int, error) {
	if len(args) == 0 {
		return 1, fmt.Errorf("exec requires a command")
	}
	incusName, err := s.resolveIncusName(ctx, name)
	if err != nil {
		return 1, err
	}
	cmd := fmt.Sprintf("incus exec %s -- %s", shellQuote(incusName), strings.Join(shellQuoteArgs(args), " "))
	opts := transport.RunOpts{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	if wantsInteractive(args) {
		err := s.Exec.RunInteractive(ctx, cmd, opts)
		if exitErr, ok := err.(*transport.ExitError); ok {
			return exitErr.Code, nil
		}
		return 0, err
	}
	return s.Exec.Run(ctx, cmd, opts)
}

func shellQuoteArgs(args []string) []string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = shellQuote(a)
	}
	return quoted
}

func wantsInteractive(args []string) bool {
	for _, a := range args {
		if a == "-it" || a == "-i" || a == "-t" || strings.HasPrefix(a, "-it") {
			return true
		}
	}
	return false
}
