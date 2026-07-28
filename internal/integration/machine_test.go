package integration_test

import (
	"context"
	"testing"

	"github.com/degoke/outpost/internal/machine"
	"github.com/degoke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

func TestMachineListIntegration(t *testing.T) {
	exec := mock.New()
	exec.Responses["incus list --format json 2>/dev/null || true"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{
		Stdout:   `[{"name":"outpost-dev","status":"Running","type":"container","state":{"network":{"eth0":{"addresses":[{"family":"inet","address":"10.0.0.5"}]}}}}]`,
		ExitCode: 0,
	}
	exec.Responses["ls -1"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: "dev\n", ExitCode: 0}
	exec.Files["/var/lib/outpost/machines/dev/meta.yaml"] = []byte(`name: dev
incus_name: outpost-dev
type: container
image: ubuntu:24.04
cpu: 1
memory_bytes: 1073741824
disk_bytes: 10737418240
`)

	svc := &machine.Service{Exec: exec, HostName: "personal"}
	list, err := svc.List(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "dev", list[0].Name)
	require.Equal(t, "running", list[0].Status)
	require.Equal(t, "10.0.0.5", list[0].IPv4)
}
