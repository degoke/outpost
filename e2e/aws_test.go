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
	mustRunOutpost(t, dir, "compose", "up", "-d", "--build")
	t.Cleanup(func() {
		_, _, _ = runOutpostAllowFail(t, dir, "compose", "down")
	})

	health := mustRunOutpost(t, dir, "compose", "exec", "-T", "app", "node", "-e", `fetch("http://127.0.0.1:3000/health").then(r=>r.text()).then(t=>{console.log(t);if(!t.includes("ok"))process.exit(1)})`)
	if !strings.Contains(health, "ok") {
		t.Fatalf("aws compose health check failed:\n%s", health)
	}
}
