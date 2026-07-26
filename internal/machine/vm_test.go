package machine_test

import (
	"context"
	"testing"

	"github.com/goke/outpost/internal/capabilities"
	"github.com/goke/outpost/internal/machine"
	"github.com/goke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

func TestCreateVMRejectedWithoutKVM(t *testing.T) {
	exec := mock.New()
	bootstrapMocks(exec)
	exec.Responses["test -c /dev/kvm"] = mockResp(1, "")
	exec.Responses["command -v incus >/dev/null 2>&1"] = mockResp(0, "")
	exec.Responses["curl -s --max-time 1 http://169.254.169.254/latest/meta-data/instance-type 2>/dev/null || true"] = mockResp(0, "")

	svc := &machine.Service{Exec: exec}
	err := svc.Create(context.Background(), "vm-dev", machine.CreateOptions{
		Image:          "ubuntu:24.04",
		VirtualMachine: true,
	}, nil)
	require.Error(t, err)
	require.False(t, exec.HasCommand("incus launch"))

	report, err := capabilities.VMSupport(context.Background(), exec, nil)
	require.NoError(t, err)
	require.False(t, report.CanCreateVM())
}

func TestCreateVMLaunchIncludesVMFlag(t *testing.T) {
	exec := mock.New()
	bootstrapMocks(exec)
	capacityMocks(exec)
	exec.Responses["test -c /dev/kvm"] = mockResp(0, "")
	exec.Responses["test -r /dev/kvm"] = mockResp(0, "")
	exec.Responses["grep -Eq 'vmx|svm' /proc/cpuinfo"] = mockResp(0, "")
	exec.Responses["command -v incus >/dev/null 2>&1"] = mockResp(0, "")
	exec.Responses["incus list --format json 2>/dev/null || true"] = mockResp(0, "[]")
	exec.Responses["ls -1"] = mockResp(0, "")
	exec.Responses["incus image info"] = mockResp(0, "")
	exec.Responses["incus launch"] = mockResp(0, "")

	svc := &machine.Service{Exec: exec}
	err := svc.Create(context.Background(), "vm-dev", machine.CreateOptions{
		Image:          "ubuntu:24.04",
		VirtualMachine: true,
	}, nil)
	require.NoError(t, err)
	require.True(t, exec.HasCommand("--vm"))
	meta, ok := exec.Uploads["/var/lib/outpost/machines/vm-dev/meta.yaml"]
	require.True(t, ok)
	require.Contains(t, string(meta), "type: vm")
}
