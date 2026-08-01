package machine_test

import (
	"context"
	"testing"

	"github.com/degoke/outpost/internal/machine"
	"github.com/degoke/outpost/internal/transport/mock"
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

func TestShellUsesInteractiveExec(t *testing.T) {
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
	require.True(t, exec.HasCommand("incus exec 'outpost-dev' -- bash"))
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
	exec.Responses["incus image copy"] = mockResp(1, "")

	svc := &machine.Service{Exec: exec}
	err := svc.Create(context.Background(), "dev", machine.CreateOptions{Image: "missing:24.04"}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to pull image")
	require.False(t, exec.HasCommand("incus launch"))
}

func TestCreatePullsImageWhenMissing(t *testing.T) {
	exec := mock.New()
	bootstrapMocks(exec)
	capacityMocks(exec)
	exec.Responses["incus list --format json 2>/dev/null || true"] = mockResp(0, "[]")
	exec.Responses["ls -1"] = mockResp(0, "")
	exec.Responses["incus image info 'local:ubuntu:24.04'"] = mockResp(1, "")
	exec.Responses["incus image copy 'images:ubuntu/24.04' local: --alias 'ubuntu:24.04'"] = mockResp(0, "")
	exec.Responses["incus launch"] = mockResp(0, "")

	svc := &machine.Service{Exec: exec}
	err := svc.Create(context.Background(), "dev", machine.CreateOptions{Image: "ubuntu:24.04"}, nil)
	require.NoError(t, err)
	require.True(t, exec.HasCommand("incus image copy"))
	require.True(t, exec.HasCommand("incus launch"))
}
