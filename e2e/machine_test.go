//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestMachineLifecycle(t *testing.T) {
	if !incusAvailable(t) {
		t.Skip("Incus is not available on this host — skipping machine e2e")
	}

	dir := copyExample(t, "machine-smoke")
	mustRunOutpost(t, dir, "init", "--no-shell")

	mustRunOutpost(t, dir, "machine", "up")
	t.Cleanup(func() {
		_, _, _ = runOutpostAllowFail(t, dir, "--yes", "machine", "down")
	})
	stdout := mustRunOutpost(t, dir, "machine", "status")
	if !strings.Contains(strings.ToLower(stdout), "running") && !strings.Contains(strings.ToLower(stdout), "status=") {
		t.Fatalf("machine status unexpected output:\n%s", stdout)
	}

	execOut := mustRunOutpost(t, dir, "machine", "exec", "--", "echo", "ok")
	if !strings.Contains(execOut, "ok") {
		t.Fatalf("machine exec output:\n%s", execOut)
	}

	mustRunOutpost(t, dir, "--yes", "machine", "down")
}
