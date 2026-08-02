//go:build e2e && aws

package e2e

import (
	"strings"
	"testing"
)

// TestAWSHostLifecycle exercises a host provisioned and destroyed by TestMain:
// provider login aws → host create → (tests) → host destroy.
func TestAWSHostLifecycle(t *testing.T) {
	if hostMode() != "aws" {
		t.Skip("OUTPOST_E2E_HOST_MODE must be aws for this test")
	}

	stdout := mustRunOutpost(t, "", "host", "list")
	if !strings.Contains(stdout, testHostName) {
		t.Fatalf("host list missing %q:\n%s", testHostName, stdout)
	}

	mustRunOutpost(t, "", "host", "verify", "--yes")
	mustRunOutpost(t, "", "host", "capabilities")
	mustRunOutpost(t, "", "status")
}

func TestAWSComposeSmoke(t *testing.T) {
	if hostMode() != "aws" {
		t.Skip("OUTPOST_E2E_HOST_MODE must be aws for this test")
	}

	dir := copyExample(t, "node-postgres-redis")
	mustRunOutpost(t, dir, "init", "--no-shell")
	mustRunOutpost(t, dir, "--yes", "compose", "up", "-d", "--build")
	t.Cleanup(func() {
		_, _, _ = runOutpostAllowFail(t, dir, "--yes", "compose", "down")
	})

	health := mustRunOutpost(t, dir, "run", "--", "curl", "-sf", "http://localhost:3000/health")
	if !strings.Contains(health, "ok") {
		t.Fatalf("aws compose health check failed:\n%s", health)
	}
}
