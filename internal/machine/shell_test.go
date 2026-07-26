package machine_test

import (
	"context"
	"testing"

	"github.com/goke/outpost/internal/machine"
	"github.com/goke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

func TestRunCommandConstruction(t *testing.T) {
	exec := mock.New()
	exec.Files["/var/lib/outpost/machines/dev/meta.yaml"] = []byte(`name: dev
incus_name: outpost-dev
type: container
`)
	exec.Responses["incus list --format json 2>/dev/null || true"] = mockResp(0, "[]")

	svc := &machine.Service{Exec: exec}
	code, err := svc.RunCommand(context.Background(), "dev", []string{"uname", "-a"})
	require.NoError(t, err)
	require.Equal(t, 0, code)
	require.True(t, exec.HasCommand("incus exec 'outpost-dev' -- 'uname' '-a'"))
}

func TestShellUsesInteractiveTTY(t *testing.T) {
	exec := mock.New()
	exec.Files["/var/lib/outpost/machines/dev/meta.yaml"] = []byte(`name: dev
incus_name: outpost-dev
type: container
`)
	exec.Responses["incus list --format json 2>/dev/null || true"] = mockResp(0, "[]")
	exec.Responses["incus exec 'outpost-dev' -- test -x /bin/bash"] = mockResp(0, "")

	svc := &machine.Service{Exec: exec}
	err := svc.Shell(context.Background(), "dev")
	require.NoError(t, err)
	require.True(t, exec.HasCommand("incus exec -t 'outpost-dev' -- bash"))
}

func TestCreateRejectsMissingImage(t *testing.T) {
	exec := mock.New()
	svc := &machine.Service{Exec: exec}
	err := svc.Create(context.Background(), "dev", machine.CreateOptions{}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "image")
}

func TestCreateRejectsUnknownImage(t *testing.T) {
	exec := mock.New()
	bootstrapMocks(exec)
	capacityMocks(exec)
	exec.Responses["incus list --format json 2>/dev/null || true"] = mockResp(0, "[]")
	exec.Responses["ls -1"] = mockResp(0, "")
	exec.Responses["incus image info"] = mockResp(1, "")

	svc := &machine.Service{Exec: exec}
	err := svc.Create(context.Background(), "dev", machine.CreateOptions{Image: "missing:24.04"}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
	require.False(t, exec.HasCommand("incus launch"))
}
