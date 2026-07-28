package mirror_test

import (
	"context"
	"testing"

	"github.com/goke/outpost/internal/config"
	"github.com/goke/outpost/internal/mirror"
	"github.com/goke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

func TestShellEnsuresRemoteDir(t *testing.T) {
	exec := mock.New()
	exec.Responses["test -d"] = struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{ExitCode: 1}
	proj := &config.Project{Name: "demo", RemoteDir: "/var/lib/outpost/projects/demo"}
	runner := mirror.New(exec, proj, "/tmp/demo", "personal")

	_ = runner.Shell(context.Background())
	require.True(t, exec.HasCommand("mkdir -p /var/lib/outpost/projects/demo"))
}
