package cluster_test

import (
	"context"
	"testing"

	"github.com/degoke/outpost/internal/capacity"
	"github.com/degoke/outpost/internal/cluster"
	"github.com/degoke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

func TestCreateRejectsInsufficientCapacity(t *testing.T) {
	exec := mock.New()
	exec.Responses["command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{ExitCode: 0}
	exec.Responses["command -v kind >/dev/null 2>&1 && command -v kubectl >/dev/null 2>&1"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{ExitCode: 0}
	exec.Responses["nproc"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: "1\n", ExitCode: 0}
	exec.Responses["free -b | head -2"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: "              total        used        free      shared  buff/cache   available\nMem:    1073741824   536870912   268435456           0   268435456   536870912\n", ExitCode: 0}
	exec.Responses["df -B1 / | tail -1"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: "1000000000 500000000 400000000 60% /\n", ExitCode: 0}
	exec.Responses["head -1 /proc/stat"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: "cpu  100 0 50 850 0 0 0 0 0 0\n", ExitCode: 0}
	exec.Responses["docker stats --no-stream --format '{{json .}}'"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: "", ExitCode: 0}
	exec.Responses["kind get clusters 2>/dev/null || true"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: "", ExitCode: 0}
	exec.Responses["ls -1 /var/lib/outpost/clusters 2>/dev/null || true"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: "", ExitCode: 0}

	svc := &cluster.Service{Exec: exec}
	err := svc.Create(context.Background(), "dev", 4, 1)
	require.Error(t, err)
	require.False(t, exec.HasCommand("kind create cluster"))
}

func TestCreateRollsBackOnKindFailure(t *testing.T) {
	exec := mock.New()
	exec.Responses["command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1"] = struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{ExitCode: 0}
	exec.Responses["command -v kind >/dev/null 2>&1 && command -v kubectl >/dev/null 2>&1"] = struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{ExitCode: 0}
	exec.Responses["nproc"] = struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{Stdout: "8\n", ExitCode: 0}
	exec.Responses["free -b | head -2"] = struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{Stdout: "              total        used        free      shared  buff/cache   available\nMem:   17179869184  2147483648 1073741824           0  1073741824 15032385536\n", ExitCode: 0}
	exec.Responses["df -B1 / | tail -1"] = struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{Stdout: "/dev/root 100000000000 10000000000 90000000000 10% /\n", ExitCode: 0}
	exec.Responses["head -1 /proc/stat"] = struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{Stdout: "cpu  100 0 50 8500 0 0 0 0 0 0\n", ExitCode: 0}
	exec.Responses["docker stats --no-stream --format '{{json .}}'"] = struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{Stdout: "", ExitCode: 0}
	exec.Responses["kind get clusters 2>/dev/null || true"] = struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{Stdout: "", ExitCode: 0}
	exec.Responses["ls -1 /var/lib/outpost/clusters 2>/dev/null || true"] = struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{Stdout: "", ExitCode: 0}
	exec.Responses["kind create cluster --name"] = struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{ExitCode: 1, Stderr: "failed"}

	svc := &cluster.Service{Exec: exec}
	err := svc.Create(context.Background(), "dev", 0, 1)
	require.Error(t, err)
	require.True(t, exec.HasCommand("kind delete cluster"))
}

func TestCapacityCheckRejectsLargeRequest(t *testing.T) {
	rep := &capacity.Report{
		AvailableCPU:  1,
		AvailableMem:  512 * 1024 * 1024,
		AvailableDisk: 1 * 1024 * 1024 * 1024,
	}
	cpu, mem, disk := cluster.EstimateResources(1, 4)
	err := capacity.CheckWithReport(rep, capacity.Request{CPUCores: cpu, MemoryBytes: mem, DiskBytes: disk})
	require.Error(t, err)
}
