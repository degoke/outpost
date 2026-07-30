package mirror_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/mirror"
	"github.com/degoke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

func TestEnsureToolchainUsesRemoteCache(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/foo\n\ngo 1.22.5\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Makefile"), []byte("build:\n\tgo build ./...\n"), 0644))

	exec := mock.New()
	plan, err := mirror.DetectPlan(dir, nil, "make build")
	require.NoError(t, err)
	state, err := json.Marshal(struct {
		Fingerprint string `json:"fingerprint"`
	}{Fingerprint: mirror.PlanFingerprintForTest(plan)})
	require.NoError(t, err)
	exec.Responses["cat /var/lib/outpost/projects/demo/.outpost/toolchain.json"] = struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{Stdout: string(state), ExitCode: 0}

	proj := &config.Project{Name: "demo", RemoteDir: "/var/lib/outpost/projects/demo"}
	runner := mirror.New(exec, proj, dir, "personal", nil)
	cmd, err := runner.EnsureToolchainForRunForTest(context.Background(), "make build", false)
	require.NoError(t, err)
	require.Contains(t, cmd, "export PATH=/var/lib/outpost/toolchains/go/1.22.5/bin:$PATH")
	require.False(t, exec.HasCommand("apt-get install"))
}

func TestEnsureToolchainSkipsWhenAutoDisabled(t *testing.T) {
	auto := false
	proj := &config.Project{
		Name:      "demo",
		RemoteDir: "/var/lib/outpost/projects/demo",
		Toolchain: &config.ProjectToolchain{Auto: &auto, Go: "1.22.5"},
	}
	runner := mirror.New(mock.New(), proj, t.TempDir(), "personal", nil)
	cmd, err := runner.EnsureToolchainForRunForTest(context.Background(), "make build", false)
	require.NoError(t, err)
	require.Equal(t, "make build", cmd)
}
