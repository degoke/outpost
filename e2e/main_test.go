//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if !enabled() {
		os.Exit(0)
	}

	var err error
	binaryPath, err = resolveBinary()
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: resolve binary: %v\n", err)
		os.Exit(1)
	}
	if _, err := os.Stat(binaryPath); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: binary not found at %s — run make build-e2e\n", binaryPath)
		os.Exit(1)
	}

	testHostName = hostName()
	mode := hostMode()

	switch mode {
	case "existing":
		if name := strings.TrimSpace(os.Getenv("OUTPOST_E2E_HOST")); name != "" {
			testHostName = name
		}
		realHome, homeErr := os.UserHomeDir()
		if homeErr != nil {
			fmt.Fprintf(os.Stderr, "e2e: existing host mode needs user home: %v\n", homeErr)
			os.Exit(1)
		}
		testConfigDir = filepath.Join(realHome, ".outpost")
		if err := verifyExistingHost(); err != nil {
			fmt.Fprintf(os.Stderr, "e2e: existing host: %v\n", err)
			os.Exit(1)
		}
	case "loopback":
		if err := initTempConfigDir(); err != nil {
			fmt.Fprintf(os.Stderr, "e2e: temp config: %v\n", err)
			os.Exit(1)
		}
		defer os.RemoveAll(tempConfigRoot)
		if err := setupLoopbackHost(); err != nil {
			fmt.Fprintf(os.Stderr, "e2e: loopback setup: %v\n", err)
			os.Exit(1)
		}
	case "aws":
		if err := initTempConfigDir(); err != nil {
			fmt.Fprintf(os.Stderr, "e2e: temp config: %v\n", err)
			os.Exit(1)
		}
		defer os.RemoveAll(tempConfigRoot)
		if err := setupAWSHost(); err != nil {
			fmt.Fprintf(os.Stderr, "e2e: aws setup: %v\n", err)
			os.Exit(1)
		}
		defer teardownAWSHost()
	default:
		fmt.Fprintf(os.Stderr, "e2e: unknown OUTPOST_E2E_HOST_MODE %q\n", mode)
		os.Exit(1)
	}

	code := m.Run()
	os.Exit(code)
}

var tempConfigRoot string

func initTempConfigDir() error {
	root, err := os.MkdirTemp("", "outpost-e2e-config-*")
	if err != nil {
		return err
	}
	tempConfigRoot = root
	testConfigDir = filepath.Join(root, ".outpost")
	if err := os.MkdirAll(testConfigDir, 0o700); err != nil {
		return err
	}
	return nil
}

func verifyExistingHost() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	stdout, stderr, err := runOutpost(ctx, "", "host", "list")
	if err != nil {
		return fmt.Errorf("host list: %w\n%s", err, stderr)
	}
	if !strings.Contains(stdout, testHostName) {
		return fmt.Errorf("host %q not found in config — register it first with outpost host add", testHostName)
	}
	_, stderr, err = runOutpost(ctx, "", "host", "verify", "--yes")
	if err != nil {
		return fmt.Errorf("host verify: %w\n%s", err, stderr)
	}
	return nil
}

func setupLoopbackHost() error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	script := filepath.Join(root, "scripts", "e2e", "setup-loopback-host.sh")
	cmd := exec.Command("bash", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, string(out))
	}
	key := strings.TrimSpace(os.Getenv("OUTPOST_E2E_SSH_KEY"))
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "OUTPOST_E2E_SSH_KEY=") {
			key = strings.TrimPrefix(line, "OUTPOST_E2E_SSH_KEY=")
			_ = os.Setenv("OUTPOST_E2E_SSH_KEY", key)
		}
	}
	if key == "" {
		return fmt.Errorf("loopback setup did not produce OUTPOST_E2E_SSH_KEY")
	}

	user := os.Getenv("USER")
	if user == "" {
		user = os.Getenv("LOGNAME")
	}
	if user == "" {
		return fmt.Errorf("could not determine SSH user for loopback host")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	_, stderr, err := runOutpost(ctx, "", "host", "add", testHostName,
		"--hostname", "127.0.0.1",
		"--user", user,
		"--auth", "key",
		"--identity-file", key,
		"--yes",
	)
	if err != nil {
		return fmt.Errorf("host add: %w\n%s", err, stderr)
	}
	_, stderr, err = runOutpost(ctx, "", "host", "verify", "--yes")
	if err != nil {
		return fmt.Errorf("host verify: %w\n%s", err, stderr)
	}
	return nil
}

func setupAWSHost() error {
	profile := resolveAWSProfile()
	region, err := resolveAWSRegion()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()

	args := []string{"provider", "login", "aws", "--region", region}
	if profile != "" {
		args = append(args, "--profile", profile)
	}
	_, stderr, err := runOutpost(ctx, "", args...)
	if err != nil {
		return fmt.Errorf("provider login: %w\n%s", err, stderr)
	}

	createArgs := []string{"host", "create", testHostName, "--provider", "aws", "--region", region}
	if profile != "" {
		createArgs = append(createArgs, "--profile", profile)
	}
	_, stderr, err = runOutpost(ctx, "", createArgs...)
	if err != nil {
		return fmt.Errorf("host create: %w\n%s", err, stderr)
	}

	_, stderr, err = runOutpost(ctx, "", "host", "update-ssh-access", testHostName)
	if err != nil {
		return fmt.Errorf("host update-ssh-access: %w\n%s", err, stderr)
	}
	_, stderr, err = runOutpost(ctx, "", "host", "verify", "--yes")
	if err != nil {
		return fmt.Errorf("host verify: %w\n%s", err, stderr)
	}
	return nil
}

func resolveAWSProfile() string {
	if profile := strings.TrimSpace(os.Getenv("OUTPOST_E2E_AWS_PROFILE")); profile != "" {
		return profile
	}
	if profile := strings.TrimSpace(os.Getenv("AWS_PROFILE")); profile != "" {
		return profile
	}
	return ""
}

func resolveAWSRegion() (string, error) {
	for _, key := range []string{"OUTPOST_E2E_AWS_REGION", "AWS_REGION", "AWS_DEFAULT_REGION"} {
		if region := strings.TrimSpace(os.Getenv(key)); region != "" {
			return region, nil
		}
	}

	args := []string{"configure", "get", "region"}
	if profile := resolveAWSProfile(); profile != "" {
		args = append(args, "--profile", profile)
	}
	cmd := exec.Command("aws", args...)
	if home, err := os.UserHomeDir(); err == nil {
		cmd.Env = append(os.Environ(), "HOME="+home)
	}
	out, err := cmd.Output()
	if err == nil {
		if region := strings.TrimSpace(string(out)); region != "" {
			return region, nil
		}
	}

	return "", fmt.Errorf("AWS region is required for e2e aws mode — set OUTPOST_E2E_AWS_REGION, AWS_REGION, or aws configure set region REGION")
}

func teardownAWSHost() {
	if os.Getenv("OUTPOST_E2E_KEEP_HOST") == "1" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	_, stderr, err := runOutpost(ctx, "", "--yes", "host", "destroy", testHostName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: aws teardown failed: %v\n%s", err, stderr)
	}
}
