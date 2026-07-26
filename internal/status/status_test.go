package status_test

import (
	"context"
	"testing"

	"github.com/goke/outpost/internal/status"
	"github.com/goke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

func TestCollectStatusJSONShape(t *testing.T) {
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
		Stdout: "              total        used        free      shared  buff/cache   available\nMem:    8589934592  4294967296  2147483648           0  2147483648  4294967296\n",
	}
	exec.Responses["df -B1"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: "/dev/sda1 100000000000 50000000000 45000000000 53% /\n"}
	exec.Responses["cat /proc/uptime"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: "3600.0 0\n"}
	exec.Responses["head -1 /proc/stat"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: "cpu  100 0 0 0 0 0 0 0 0 0"}
	exec.Responses["docker info"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{}
	exec.Responses["docker ps"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: `{"ID":"abc","Names":"web-1","Image":"nginx","State":"running","Status":"Up","Labels":"com.docker.compose.project=demo"}`}
	exec.Responses["docker images -q"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: "1\n"}
	exec.Responses["docker volume ls -q"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: "0\n"}
	exec.Responses["docker system df"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{
		Stdout: "TYPE            TOTAL     ACTIVE    SIZE      RECLAIMABLE\nImages          1         1         1GB       0B (0%)\n",
	}
	exec.Responses["docker compose ls"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: `{"Name":"demo","Status":"running(1)"}`}

	exec.Responses["kind get clusters 2>/dev/null || true"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: "", ExitCode: 0}
	exec.Responses["ls -1"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: "", ExitCode: 0}
	exec.Responses["incus list --format json 2>/dev/null || true"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: `[{"name":"outpost-dev","status":"Running","type":"container","state":{}}]`, ExitCode: 0}

	report, err := status.Collect(context.Background(), exec)
	require.NoError(t, err)
	require.Equal(t, 4, report.Host.CPUCores)
	require.True(t, report.Docker.Healthy)
	require.Equal(t, 1, report.Docker.ContainersRun)
	require.Len(t, report.Compose, 1)
	require.Equal(t, 0, report.Clusters)
	require.Equal(t, 1, report.Machines)
}
