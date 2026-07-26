//go:build !darwin && !linux

package transport

import "golang.org/x/crypto/ssh"

func isTerminal(fd int) bool {
	return false
}

func requestPTY(session *ssh.Session) error {
	return nil
}
