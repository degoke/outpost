package transport

import (
	"context"
	"fmt"
	"io"
	"strings"
)

type RunOpts struct {
	WorkDir string
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
}

type ForwardSpec struct {
	LocalHost  string
	LocalPort  int
	RemoteHost string
	RemotePort int
}

type Executor interface {
	Run(ctx context.Context, cmd string, opts RunOpts) (exitCode int, err error)
	RunInteractive(ctx context.Context, cmd string, opts RunOpts) error
	Upload(local, remote string) error
	UploadBytes(data []byte, remote string) error
	Download(remote string) ([]byte, error)
	Forward(ctx context.Context, spec ForwardSpec) (io.Closer, error)
	HostInfo() string
}

type SSHConfig struct {
	Hostname         string
	User             string
	Port             int
	IdentityFile     string
	Password         string
	Passphrase       []byte
	PromptAuth       bool
	AuthMode         AuthMode
	AutoTrustHostKey bool
}

// AuthMode selects how Outpost authenticates over SSH.
type AuthMode string

const (
	// AuthAuto uses --identity-file when set; otherwise password auth only.
	AuthAuto AuthMode = "auto"
	// AuthPassword uses the server login password (no local SSH key).
	AuthPassword AuthMode = "password"
	// AuthKey uses a private key file (and optional key passphrase).
	AuthKey AuthMode = "key"
)

func ParseAuthMode(s string) (AuthMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", string(AuthAuto):
		return AuthAuto, nil
	case string(AuthPassword):
		return AuthPassword, nil
	case string(AuthKey):
		return AuthKey, nil
	default:
		return "", fmt.Errorf("unknown auth mode %q (use auto, password, or key)", s)
	}
}

func (c SSHConfig) String() string {
	return fmt.Sprintf("%s@%s:%d", c.User, c.Hostname, c.Port)
}
