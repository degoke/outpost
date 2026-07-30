package mirror_test

import (
	"context"
	"testing"

	"github.com/degoke/outpost/internal/mirror"
	"github.com/degoke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

func TestEnsureToolchainInstallsMake(t *testing.T) {
	exec := mock.New()
	exec.Responses["command -v make"] = struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{ExitCode: 1}
	exec.Responses["apt-get"] = struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{ExitCode: 0}

	_, err := mirror.EnsureToolchain(context.Background(), exec, mirror.ToolchainPlan{
		Packages: []string{"make"},
	}, nil)
	require.NoError(t, err)
	require.True(t, exec.HasCommand("apt-get install"))
}

func TestEnsureToolchainSkipsInstalledGo(t *testing.T) {
	exec := mock.New()
	exec.Responses["test -x /var/lib/outpost/toolchains/go/1.22.5/bin/go"] = struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{ExitCode: 0}

	paths, err := mirror.EnsureToolchain(context.Background(), exec, mirror.ToolchainPlan{
		GoVersion: "1.22.5",
	}, nil)
	require.NoError(t, err)
	require.Len(t, paths, 1)
	require.False(t, exec.HasCommand("curl -fsSL https://go.dev/dl/"))
}
