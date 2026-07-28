package cluster

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/degoke/outpost/internal/transport"
)

func RunKubectl(ctx context.Context, exec transport.Executor, clusterName string, args []string) (int, error) {
	safe := strings.TrimSpace(clusterName)
	if safe == "" {
		return 1, fmt.Errorf("--cluster is required")
	}
	kubeconfig := RemoteKubeconfig(safe)
	uploadsDir := RemoteDir(safe) + "/uploads"
	resolved, err := resolveLocalFiles(args, uploadsDir, exec)
	if err != nil {
		return 1, err
	}
	cmd := fmt.Sprintf("KUBECONFIG=%s kubectl %s", shellQuote(kubeconfig), strings.Join(shellQuoteArgs(resolved), " "))
	opts := transport.RunOpts{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	if wantsInteractive(resolved) {
		err := exec.RunInteractive(ctx, cmd, opts)
		if exitErr, ok := err.(*transport.ExitError); ok {
			return exitErr.Code, nil
		}
		return 0, err
	}
	return exec.Run(ctx, cmd, opts)
}

func resolveLocalFiles(args []string, uploadsDir string, exec transport.Executor) ([]string, error) {
	out := make([]string, len(args))
	copy(out, args)
	for i := 0; i < len(out); i++ {
		if out[i] == "-f" || out[i] == "--filename" {
			if i+1 >= len(out) {
				continue
			}
			path := out[i+1]
			if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
				continue
			}
			if _, err := os.Stat(path); err != nil {
				continue
			}
			abs, err := filepath.Abs(path)
			if err != nil {
				return nil, err
			}
			remote := uploadsDir + "/" + filepath.Base(abs)
			if err := transport.EnsureRemoteDir(exec, uploadsDir); err != nil {
				return nil, err
			}
			if err := exec.Upload(abs, remote); err != nil {
				return nil, fmt.Errorf("upload manifest %s: %w", path, err)
			}
			out[i+1] = remote
		}
	}
	return out, nil
}

// ResolveLocalFilesForTest exposes manifest upload resolution for tests.
func ResolveLocalFilesForTest(args []string, uploadsDir string, exec transport.Executor) ([]string, error) {
	return resolveLocalFiles(args, uploadsDir, exec)
}

func shellQuoteArgs(args []string) []string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = shellQuote(a)
	}
	return quoted
}

func wantsInteractive(args []string) bool {
	for _, a := range args {
		if a == "-it" || a == "-i" || a == "-t" || strings.HasPrefix(a, "-it") {
			return true
		}
	}
	return false
}
