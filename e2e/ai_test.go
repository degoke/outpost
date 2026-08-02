//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestAISmokeRun(t *testing.T) {
	dir := copyExample(t, "ai-smoke")
	mustRunOutpost(t, dir, "init", "--no-shell")

	stdout := mustRunOutpost(t, dir, "run", "--", "sh", "-c", "echo e2e-ok")
	if !strings.Contains(stdout, "e2e-ok") {
		t.Fatalf("ai-smoke run output:\n%s", stdout)
	}
}
