//go:build darwin || linux

package transport

import (
	"os"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

func isTerminal(fd int) bool {
	return term.IsTerminal(fd)
}

func requestPTY(session *ssh.Session) error {
	w, h, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		w, h = 80, 24
	}
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	return session.RequestPty("xterm-256color", h, w, modes)
}
