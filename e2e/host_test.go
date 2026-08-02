//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestHostCommands(t *testing.T) {
	stdout := mustRunOutpost(t, "", "host", "list")
	if !strings.Contains(stdout, testHostName) {
		t.Fatalf("host list missing %q:\n%s", testHostName, stdout)
	}

	mustRunOutpost(t, "", "host", "verify", "--yes")
	mustRunOutpost(t, "", "host", "capabilities")
	mustRunOutpost(t, "", "status")
	mustRunOutpost(t, "", "capacity")
	mustRunOutpost(t, "", "disk")
	mustRunOutpost(t, "", "prune", "--dry-run")
	mustRunOutpost(t, "", "invite", "list")
}
