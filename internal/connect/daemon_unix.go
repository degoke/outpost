//go:build unix

package connect

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func SpawnDetached(argv []string, extraEnv []string) (int, error) {
	if len(argv) == 0 {
		return 0, fmt.Errorf("missing executable")
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	go func() { _ = cmd.Wait() }()
	return cmd.Process.Pid, nil
}
