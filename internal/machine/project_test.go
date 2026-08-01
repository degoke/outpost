package machine_test

import (
	"context"
	"testing"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/machine"
	"github.com/degoke/outpost/internal/output"
	"github.com/degoke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

func TestProjectServiceUsesProjectRemoteDirectory(t *testing.T) {
	exec := mock.New()
	exec.Files["/var/lib/outpost/projects/demo/.outpost/machines/demo/meta.yaml"] = []byte(`name: demo
incus_name: outpost-demo
type: container
image: ubuntu:24.04
cpu: 0.5
memory_bytes: 134217728
disk_bytes: 2147483648
`)
	exec.Responses["incus list --format json"] = mockResp(0, `[{"name":"outpost-demo","status":"Running","type":"container","state":{"network":{"eth0":{"addresses":[{"family":"inet","address":"10.0.0.5"}]}}}}]`)

	service := machine.NewProjectService(exec, &config.Project{
		Name:      "demo",
		RemoteDir: "/var/lib/outpost/projects/demo",
	}, output.New(true, false))
	status, err := service.Status(context.Background())
	require.NoError(t, err)
	require.Equal(t, "demo", status.Name)
	require.Equal(t, "running", status.Status)
	require.Equal(t, "10.0.0.5", status.IPv4)
}
