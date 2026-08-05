//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestClusterLifecycle(t *testing.T) {
	dir := copyExample(t, "cluster-smoke")
	mustRunOutpost(t, dir, "init", "--no-shell")

	mustRunOutpost(t, dir, "cluster", "up")
	t.Cleanup(func() {
		_, _, _ = runOutpostAllowFail(t, dir, "--yes", "cluster", "down")
	})
	stdout := mustRunOutpost(t, dir, "cluster", "status")
	if !strings.Contains(strings.ToLower(stdout), "kind") && !strings.Contains(strings.ToLower(stdout), "running") && !strings.Contains(strings.ToLower(stdout), "ready") {
		t.Fatalf("cluster status unexpected output:\n%s", stdout)
	}

	// Use --foreground so e2e (non-TTY) runs kubectl directly without tmux.
	nodes := mustRunOutpost(t, dir, "run", "--foreground", "--", "kubectl", "get", "nodes", "--no-headers")
	if strings.TrimSpace(nodes) == "" {
		t.Fatalf("kubectl get nodes returned empty output")
	}

	mustRunOutpost(t, dir, "--yes", "cluster", "down")
}
