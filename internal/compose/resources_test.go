package compose_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/degoke/outpost/internal/capacity"
	"github.com/degoke/outpost/internal/compose"
	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/inspect"
	"github.com/degoke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

func TestCheckComposeCapacityParsesLimits(t *testing.T) {
	dir := t.TempDir()
	composeFile := `services:
  web:
    deploy:
      resources:
        limits:
          cpus: 2
          memory: 512m
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(composeFile), 0644))
	proj := &config.Project{ComposeFiles: []string{"docker-compose.yml"}}

	exec := mock.New()
	exec.Responses["nproc"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: "4\n"}
	exec.Responses["free -b"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{
		Stdout: "              total        used        free      shared  buff/cache   available\nMem:    8589934592  8589934592           0           0           0           0\n",
	}
	exec.Responses["head -1 /proc/stat"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: "cpu  100 0 0 0 0 0 0 0 0 0"}
	exec.Responses["df -B1"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: "/dev/sda1 1000 900 100 90% /\n"}
	exec.Responses["cat /proc/uptime"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: "1000.0 0\n"}
	exec.Responses["docker stats"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: ""}

	err := compose.CheckComposeCapacity(context.Background(), exec, dir, proj)
	require.Error(t, err)
	require.Contains(t, err.Error(), "insufficient memory")
	_ = capacity.Request{}
	_ = inspect.HostMetrics{}
}
