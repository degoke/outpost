package transport

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func hostKeyCallback(cfg SSHConfig) (ssh.HostKeyCallback, error) {
	path, err := knownHostsPath()
	if err != nil {
		return nil, err
	}

	var fileCallback ssh.HostKeyCallback
	if _, err := os.Stat(path); err == nil {
		fileCallback, err = knownhosts.New(path)
		if err != nil {
			return nil, err
		}
	}

	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		if fileCallback != nil {
			err := fileCallback(hostname, remote, key)
			if err == nil {
				return nil
			}
			if isHostKeyChanged(err) {
				return fmt.Errorf("host key for %s has changed — verify the server identity before updating %s: %w", cfg.Hostname, path, err)
			}
			if !isUnknownHostKey(err) {
				return err
			}
		}
		return acceptUnknownHostKey(cfg, path, hostname, remote, key)
	}, nil
}

func knownHostsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ssh", "known_hosts"), nil
}

func isUnknownHostKey(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "key is unknown") ||
		strings.Contains(msg, "not in known_hosts") ||
		strings.Contains(msg, "no hostkey for") {
		return true
	}
	var keyErr *knownhosts.KeyError
	if errors.As(err, &keyErr) && keyErr != nil && len(keyErr.Want) == 0 {
		return true
	}
	return false
}

// IsUnknownHostKeyForTest exposes unknown-host detection for tests.
func IsUnknownHostKeyForTest(err error) bool {
	return isUnknownHostKey(err)
}

func isHostKeyChanged(err error) bool {
	var keyErr *knownhosts.KeyError
	if !errors.As(err, &keyErr) || keyErr == nil {
		return false
	}
	return len(keyErr.Want) > 0
}

func acceptUnknownHostKey(cfg SSHConfig, knownHostsPath, hostname string, remote net.Addr, key ssh.PublicKey) error {
	display := hostDisplayName(cfg, remote)
	fingerprint := ssh.FingerprintSHA256(key)

	if cfg.AutoTrustHostKey {
		fmt.Fprintf(os.Stderr, "Trusting new host key for %s (%s)\n", display, fingerprint)
		return appendKnownHost(knownHostsPath, hostname, remote, key)
	}

	if !cfg.PromptAuth || !IsInteractive() {
		return fmt.Errorf("host key for %s is unknown — run interactively to accept it, add it to %s, or pass --yes", display, knownHostsPath)
	}

	fmt.Fprintf(os.Stderr, "The authenticity of host %s can't be established.\n", display)
	fmt.Fprintf(os.Stderr, "%s key fingerprint is %s.\n", key.Type(), fingerprint)
	fmt.Fprintf(os.Stderr, "Are you sure you want to continue connecting (yes/no)? ")
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("aborted")
	}
	line = strings.TrimSpace(strings.ToLower(line))
	if line != "y" && line != "yes" {
		return fmt.Errorf("aborted")
	}
	return appendKnownHost(knownHostsPath, hostname, remote, key)
}

func hostDisplayName(cfg SSHConfig, remote net.Addr) string {
	if cfg.Port != 0 && cfg.Port != 22 {
		return fmt.Sprintf("[%s]:%d", cfg.Hostname, cfg.Port)
	}
	if remote != nil {
		return remote.String()
	}
	return cfg.Hostname
}

func appendKnownHost(path, hostname string, remote net.Addr, key ssh.PublicKey) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	addresses := []string{hostname}
	if remote != nil {
		if host, port, err := net.SplitHostPort(remote.String()); err == nil && port != "22" {
			addresses = []string{fmt.Sprintf("[%s]:%s", host, port)}
		}
	}
	line := knownhosts.Line(addresses, key)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(line + "\n"); err != nil {
		return err
	}
	return nil
}
