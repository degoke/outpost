package capabilities_test

import (
	"context"
	"testing"

	"github.com/degoke/outpost/internal/capabilities"
	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

func TestVMSupportSupported(t *testing.T) {
	exec := mock.New()
	exec.Responses["test -c /dev/kvm"] = mockResp(0)
	exec.Responses["test -r /dev/kvm"] = mockResp(0)
	exec.Responses["grep -Eq 'vmx|svm' /proc/cpuinfo"] = mockResp(0)
	exec.Responses["command -v incus >/dev/null 2>&1"] = mockResp(0)

	report, err := capabilities.VMSupport(context.Background(), exec, nil)
	require.NoError(t, err)
	require.True(t, report.CanCreateVM())
	require.Equal(t, "supported", report.Status)
}

func TestVMSupportNestedPossibleOnAWS(t *testing.T) {
	exec := mock.New()
	exec.Responses["test -c /dev/kvm"] = mockResp(1)
	exec.Responses["command -v incus >/dev/null 2>&1"] = mockResp(0)
	exec.Responses["curl -s --max-time 1 http://169.254.169.254/latest/meta-data/instance-type 2>/dev/null || true"] = mockResp(0, "t3.medium\n")

	report, err := capabilities.VMSupport(context.Background(), exec, nil)
	require.NoError(t, err)
	require.False(t, report.CanCreateVM())
	require.Equal(t, "nested_possible", report.Status)
	require.NotEmpty(t, report.Remediation)
}

func TestVMSupportKVMNotReadable(t *testing.T) {
	exec := mock.New()
	exec.Responses["test -c /dev/kvm"] = mockResp(0)
	exec.Responses["test -r /dev/kvm"] = mockResp(1)
	exec.Responses["command -v incus >/dev/null 2>&1"] = mockResp(0)

	report, err := capabilities.VMSupport(context.Background(), exec, nil)
	require.NoError(t, err)
	require.False(t, report.CanCreateVM())
	require.Contains(t, report.Reason, "not readable")
}

func TestVMSupportUsesProviderMeta(t *testing.T) {
	exec := mock.New()
	exec.Responses["test -c /dev/kvm"] = mockResp(1)
	exec.Responses["command -v incus >/dev/null 2>&1"] = mockResp(0)

	report, err := capabilities.VMSupport(context.Background(), exec, &config.ProviderMeta{InstanceType: "m5.metal"})
	require.NoError(t, err)
	require.Equal(t, "nested_possible", report.Status)
}

func TestCreateVMRejectedWithoutKVM(t *testing.T) {
	exec := mock.New()
	exec.Responses["command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1"] = mockResp(0)
	exec.Responses["command -v incus >/dev/null 2>&1 && incus list >/dev/null 2>&1"] = mockResp(0)
	exec.Responses["test -c /dev/kvm"] = mockResp(1)
	exec.Responses["command -v incus >/dev/null 2>&1"] = mockResp(0)
	exec.Responses["curl -s --max-time 1 http://169.254.169.254/latest/meta-data/instance-type 2>/dev/null || true"] = mockResp(0, "")

	// Import machine package in separate test file to avoid cycle - test via VMSupport gate only
	report, err := capabilities.VMSupport(context.Background(), exec, nil)
	require.NoError(t, err)
	require.False(t, report.CanCreateVM())
}

func mockResp(code int, stdout ...string) struct {
	Stdout, Stderr string
	ExitCode       int
	Err            error
} {
	out := ""
	if len(stdout) > 0 {
		out = stdout[0]
	}
	return struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{Stdout: out, ExitCode: code}
}
