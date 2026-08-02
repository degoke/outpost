//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectInitAndRun(t *testing.T) {
	dir := t.TempDir()
	compose := `services:
  web:
    image: nginx:alpine
    ports:
      - "8080:80"
`
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(compose), 0o644); err != nil {
		t.Fatal(err)
	}

	mustRunOutpost(t, dir, "init", "--no-shell", "--name", "e2e-init")
	data, err := os.ReadFile(filepath.Join(dir, ".outpost", "project.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "name: e2e-init") {
		t.Fatalf("project.yaml missing name:\n%s", data)
	}

	mustRunOutpost(t, dir, "docker", "ps")
}

func TestProjectRunInExample(t *testing.T) {
	dir := copyExample(t, "ai-smoke")
	mustRunOutpost(t, dir, "init", "--no-shell")

	stdout := mustRunOutpost(t, dir, "run", "--", "sh", "-c", "echo e2e-ok")
	if !strings.Contains(stdout, "e2e-ok") {
		t.Fatalf("run output missing marker:\n%s", stdout)
	}
}
