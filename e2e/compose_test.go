//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"
)

func TestComposeLifecycle(t *testing.T) {
	dir := copyExample(t, "node-postgres-redis")

	mustRunOutpost(t, dir, "init", "--no-shell")
	mustRunOutpost(t, dir, "--yes", "compose", "up", "-d", "--build")
	t.Cleanup(func() {
		_, _, _ = runOutpostAllowFail(t, dir, "--yes", "compose", "down")
	})

	stdout := mustRunOutpost(t, dir, "compose", "ps")
	if !strings.Contains(stdout, "app") && !strings.Contains(strings.ToLower(stdout), "running") {
		t.Fatalf("compose ps unexpected output:\n%s", stdout)
	}

	mustRunOutpost(t, dir, "compose", "logs", "--tail", "20")

	health := mustRunOutpost(t, dir, "run", "--", "curl", "-sf", "http://localhost:3000/health")
	if !strings.Contains(health, "ok") {
		t.Fatalf("health check failed:\n%s", health)
	}

	mustRunOutpost(t, dir, "--yes", "compose", "down")
}

func TestComposeOpenHealth(t *testing.T) {
	dir := copyExample(t, "node-postgres-redis")
	mustRunOutpost(t, dir, "init", "--no-shell")
	mustRunOutpost(t, dir, "--yes", "compose", "up", "-d", "--build")
	t.Cleanup(func() {
		_, _, _ = runOutpostAllowFail(t, dir, "--yes", "compose", "down")
	})

	mustRunOutpost(t, dir, "open", "--port", "3000:3000")
	t.Cleanup(func() {
		_, _, _ = runOutpostAllowFail(t, dir, "close")
	})

	waitHTTP(t, "http://127.0.0.1:3000/health", 3*time.Minute)
	mustRunOutpost(t, dir, "close")
}
