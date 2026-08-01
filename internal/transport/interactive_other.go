//go:build !darwin && !linux

package transport

import (
	"context"
	"io"
	"os"

	"golang.org/x/crypto/ssh"
)

func runInteractiveSession(ctx context.Context, session *ssh.Session, cmd string, stdin io.Reader, stdout, stderr io.Writer) error {
	if stdin == nil {
		stdin = os.Stdin
	}
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}

	fd := int(os.Stdin.Fd())
	if isTerminal(fd) {
		if err := requestPTY(session); err != nil {
			return err
		}
	}
	session.Stdin = stdin
	session.Stdout = stdout
	session.Stderr = stderr

	if err := session.Start(cmd); err != nil {
		return err
	}
	errCh := make(chan error, 1)
	go func() { errCh <- session.Wait() }()
	select {
	case err := <-errCh:
		return exitError(err)
	case <-ctx.Done():
		_ = session.Close()
		<-errCh
		return ctx.Err()
	}
}

func exitError(err error) error {
	if err == nil {
		return nil
	}
	if exitErr, ok := err.(*ssh.ExitError); ok {
		return &ExitError{Code: exitErr.ExitStatus()}
	}
	return err
}
