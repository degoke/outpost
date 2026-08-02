//go:build e2e

// Package e2e runs real-infra tests against the outpost CLI.
//
// Required: OUTPOST_E2E=1
//
// Environment:
//
//	OUTPOST_E2E=1                  — safety gate (required)
//	OUTPOST_E2E_BINARY             — CLI path (default bin/outpost-e2e)
//	OUTPOST_E2E_HOST_MODE          — loopback | aws | existing (default loopback)
//	OUTPOST_E2E_HOST               — registered host name when mode=existing
//	OUTPOST_E2E_HOST_NAME          — host name to create/register (default ci)
//	OUTPOST_E2E_SSH_KEY            — private key for loopback host add
//	OUTPOST_E2E_KEEP_HOST=1        — skip AWS destroy on teardown
//	OUTPOST_E2E_AWS_PROFILE        — AWS profile (or: make e2e-aws PROFILE=name)
//	OUTPOST_E2E_AWS_REGION         — AWS region (or: make e2e-aws REGION=name; falls back to aws configure get region)
//	OUTPOST_E2E_STATE_DIR          — temp state for loopback SSH keys
package e2e

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const defaultHostName = "ci"

var (
	testConfigDir string
	testHostName  string
	binaryPath    string
)

func enabled() bool {
	return os.Getenv("OUTPOST_E2E") == "1"
}

func repoRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("could not resolve e2e package path")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above %s", file)
		}
		dir = parent
	}
}

func resolveBinary() (string, error) {
	if path := strings.TrimSpace(os.Getenv("OUTPOST_E2E_BINARY")); path != "" {
		return path, nil
	}
	root, err := repoRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "bin", "outpost-e2e"), nil
}

func hostName() string {
	if name := strings.TrimSpace(os.Getenv("OUTPOST_E2E_HOST_NAME")); name != "" {
		return name
	}
	return defaultHostName
}

func runOutpost(ctx context.Context, dir string, args ...string) (stdout, stderr string, err error) {
	cmd := exec.CommandContext(ctx, binaryPath, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"OUTPOST_CONFIG_DIR="+testConfigDir,
	)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	runErr := cmd.Run()
	return outBuf.String(), errBuf.String(), runErr
}

func mustRunOutpost(t *testing.T, dir string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	stdout, stderr, err := runOutpost(ctx, dir, args...)
	if err != nil {
		t.Fatalf("outpost %s failed: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), err, stdout, stderr)
	}
	return stdout
}

func runOutpostAllowFail(t *testing.T, dir string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	return runOutpost(ctx, dir, args...)
}

func copyExample(t *testing.T, name string) string {
	t.Helper()
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(root, "examples", name)
	dst := t.TempDir()
	if err := copyDir(src, dst); err != nil {
		t.Fatalf("copy example %s: %v", name, err)
	}
	return dst
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func waitHTTP(t *testing.T, url string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 5 * time.Second}
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return
			}
			lastErr = fmt.Errorf("status %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("timed out waiting for %s: %v", url, lastErr)
}

func hostMode() string {
	if name := strings.TrimSpace(os.Getenv("OUTPOST_E2E_HOST")); name != "" {
		return "existing"
	}
	mode := strings.TrimSpace(os.Getenv("OUTPOST_E2E_HOST_MODE"))
	if mode == "" {
		return "loopback"
	}
	return mode
}

func incusAvailable(t *testing.T) bool {
	t.Helper()
	stdout, stderr, err := runOutpostAllowFail(t, "", "host", "capabilities")
	if err != nil {
		t.Logf("host capabilities failed: %v\n%s%s", err, stdout, stderr)
		return false
	}
	lower := strings.ToLower(stdout)
	return strings.Contains(lower, "incus: available") || strings.Contains(lower, "incus:available")
}
