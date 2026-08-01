//go:build darwin || linux

package transport

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

func runInteractiveSession(ctx context.Context, session *ssh.Session, cmd string, stdin io.Reader, stdout, stderr io.Writer) error {
	fd := int(os.Stdin.Fd())
	useTTY := isTerminal(fd) && stdin == os.Stdin

	var oldState *term.State
	if useTTY {
		var err error
		oldState, err = term.MakeRaw(fd)
		if err != nil {
			return err
		}
		defer func() {
			_ = term.Restore(fd, oldState)
			_, _ = fmt.Fprint(stdout, "\r\n")
		}()
		if err := requestPTY(session); err != nil {
			return err
		}

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGWINCH)
		defer signal.Stop(sigCh)

		done := make(chan struct{})
		defer close(done)

		go func() {
			for {
				select {
				case <-done:
					return
				case sig := <-sigCh:
					switch sig {
					case syscall.SIGWINCH:
						if w, h, err := term.GetSize(fd); err == nil {
							_ = session.WindowChange(h, w)
						}
					case syscall.SIGTERM:
						_ = session.Close()
					default:
						_ = session.Signal(ssh.SIGINT)
					}
				}
			}
		}()
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
