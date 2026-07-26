package machine_test

import (
	"context"
	"testing"

	"github.com/goke/outpost/internal/capacity"
	"github.com/goke/outpost/internal/machine"
	"github.com/goke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

func bootstrapMocks(exec *mock.Executor) {
	exec.Responses["command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1"] = mockResp(0, "")
	exec.Responses["command -v incus >/dev/null 2>&1 && incus list >/dev/null 2>&1"] = mockResp(0, "")
}

func capacityMocks(exec *mock.Executor) {
	exec.Responses["nproc"] = mockResp(0, "8\n")
	exec.Responses["free -b | head -2"] = mockResp(0, "              total        used        free      shared  buff/cache   available\nMem:   17179869184  2147483648 1073741824           0  1073741824 15032385536\n")
	exec.Responses["df -B1 / | tail -1"] = mockResp(0, "/dev/root 100000000000 10000000000 90000000000 10% /\n")
	exec.Responses["head -1 /proc/stat"] = mockResp(0, "cpu  100 0 50 8500 0 0 0 0 0 0\n")
	exec.Responses["docker stats --no-stream --format '{{json .}}'"] = mockResp(0, "")
}

func mockResp(code int, stdout string) struct {
	Stdout, Stderr string
	ExitCode       int
	Err            error
} {
	return struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{Stdout: stdout, ExitCode: code}
}

func TestCreateRejectsInsufficientCapacity(t *testing.T) {
	exec := mock.New()
	bootstrapMocks(exec)
	capacityMocks(exec)
	exec.Responses["incus list --format json 2>/dev/null || true"] = mockResp(0, "[]")
	exec.Responses["ls -1"] = mockResp(0, "")

	svc := &machine.Service{Exec: exec}
	err := svc.Create(context.Background(), "dev", machine.CreateOptions{Image: "ubuntu:24.04", CPU: 16, MemoryBytes: 64 * 1024 * 1024 * 1024}, nil)
	require.Error(t, err)
	require.False(t, exec.HasCommand("incus launch"))
}

func TestCreateRollsBackOnLaunchFailure(t *testing.T) {
	exec := mock.New()
	bootstrapMocks(exec)
	capacityMocks(exec)
	exec.Responses["incus list --format json 2>/dev/null || true"] = mockResp(0, "[]")
	exec.Responses["ls -1"] = mockResp(0, "")
	exec.Responses["incus image info"] = mockResp(0, "")
	exec.Responses["incus launch"] = mockResp(1, "")

	svc := &machine.Service{Exec: exec}
	err := svc.Create(context.Background(), "dev", machine.CreateOptions{Image: "ubuntu:24.04"}, nil)
	require.Error(t, err)
	require.True(t, exec.HasCommand("incus delete"))
}

func TestCreateRejectsDuplicate(t *testing.T) {
	exec := mock.New()
	bootstrapMocks(exec)
	capacityMocks(exec)
	exec.Responses["incus list --format json 2>/dev/null || true"] = mockResp(0, "[]")
	exec.Files["/var/lib/outpost/machines/dev/meta.yaml"] = []byte(`name: dev
incus_name: outpost-dev
type: container
image: ubuntu:24.04
cpu: 1
memory_bytes: 1073741824
disk_bytes: 10737418240
`)
	exec.Responses["ls -1"] = mockResp(0, "dev")

	svc := &machine.Service{Exec: exec}
	err := svc.Create(context.Background(), "dev", machine.CreateOptions{Image: "ubuntu:24.04"}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "already exists")
}

func TestCapacityCheckRejectsLargeRequest(t *testing.T) {
	rep := &capacity.Report{
		AvailableCPU:  1,
		AvailableMem:  512 * 1024 * 1024,
		AvailableDisk: 1 * 1024 * 1024 * 1024,
	}
	cpu, mem, disk := machine.EstimateResources(machine.CreateOptions{VirtualMachine: true})
	err := capacity.CheckWithReport(rep, capacity.Request{CPUCores: cpu, MemoryBytes: mem, DiskBytes: disk})
	require.Error(t, err)
}

func TestParseSize(t *testing.T) {
	b, err := machine.ParseSize("2GiB")
	require.NoError(t, err)
	require.Equal(t, uint64(2*1024*1024*1024), b)
}

func TestTypeLabel(t *testing.T) {
	require.Contains(t, machine.TypeLabel(machine.TypeContainer), "shared kernel")
	require.Contains(t, machine.TypeLabel(machine.TypeVM), "hardware isolated")
}
