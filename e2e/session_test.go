//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestSessionDetachedWorkflow(t *testing.T) {
	dir := copyExample(t, "ai-smoke")
	mustRunOutpost(t, dir, "init", "--no-shell")

	mustRunOutpost(t, dir, "run", "--name", "e2e-session", "--detach", "--", "sh", "-c", "echo e2e-session-ok; sleep 120")

	listOut := mustRunOutpost(t, dir, "session", "list")
	if !strings.Contains(listOut, "e2e-session") {
		t.Fatalf("session list missing e2e-session:\n%s", listOut)
	}

	statusOut := mustRunOutpost(t, dir, "session", "status", "e2e-session")
	if !strings.Contains(statusOut, "running") {
		t.Fatalf("session status expected running:\n%s", statusOut)
	}

	logsOut := mustRunOutpost(t, dir, "session", "logs", "e2e-session", "--tail", "20")
	if !strings.Contains(logsOut, "e2e-session-ok") {
		t.Fatalf("session logs missing marker:\n%s", logsOut)
	}

	mustRunOutpost(t, dir, "session", "kill", "e2e-session")

	listAfter := mustRunOutpost(t, dir, "session", "list")
	if strings.Contains(listAfter, "e2e-session") && strings.Contains(listAfter, "running") {
		t.Fatalf("session still listed as running after kill:\n%s", listAfter)
	}
}
