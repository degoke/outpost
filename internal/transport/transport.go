package transport

import (
	"context"
	"fmt"
	"io"
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
	Hostname     string
	User         string
	Port         int
	IdentityFile string
}

func (c SSHConfig) String() string {
	return fmt.Sprintf("%s@%s:%d", c.User, c.Hostname, c.Port)
}
