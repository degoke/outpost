package transport

import (
	"context"
	"fmt"
)

func (e *SSHExecutor) RunMkdir(dir string) error {
	cmd := fmt.Sprintf("mkdir -p %s", shellQuote(dir))
	_, err := e.Run(context.Background(), cmd, RunOpts{})
	return err
}

func EnsureRemoteDir(exec Executor, dir string) error {
	if m, ok := exec.(interface{ RunMkdir(string) error }); ok {
		return m.RunMkdir(dir)
	}
	cmd := fmt.Sprintf("mkdir -p %s", shellQuote(dir))
	_, err := exec.Run(context.Background(), cmd, RunOpts{})
	return err
}
