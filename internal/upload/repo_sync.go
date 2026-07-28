package upload

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/transport"
)

func SyncRepo(cwd string, proj *config.Project, exec transport.Executor) error {
	if err := transport.EnsureRemoteDir(exec, proj.RemoteDir); err != nil {
		return err
	}
	files, err := collectRepoPaths(cwd)
	if err != nil {
		return err
	}
	for _, rel := range files {
		local := filepath.Join(cwd, rel)
		remote := remotePath(proj, rel)
		if err := syncFile(exec, local, remote); err != nil {
			return err
		}
	}
	return nil
}

func collectRepoPaths(cwd string) ([]string, error) {
	if isGitRepo(cwd) {
		return gitRepoPaths(cwd)
	}
	return walkRepoPaths(cwd)
}

func isGitRepo(cwd string) bool {
	_, err := os.Stat(filepath.Join(cwd, ".git"))
	return err == nil
}

func gitRepoPaths(cwd string) ([]string, error) {
	cmd := exec.Command("git", "ls-files", "-co", "--exclude-standard")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		paths = append(paths, filepath.ToSlash(line))
	}
	return paths, nil
}

func walkRepoPaths(cwd string) ([]string, error) {
	ignorePatterns := loadOutpostIgnore(cwd)
	var paths []string
	err := filepath.WalkDir(cwd, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(cwd, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if shouldIgnoreRepo(rel, ignorePatterns, true) {
				return filepath.SkipDir
			}
			return nil
		}
		if shouldIgnoreRepo(rel, ignorePatterns, false) {
			return nil
		}
		paths = append(paths, rel)
		return nil
	})
	return paths, err
}

func loadOutpostIgnore(cwd string) []string {
	patterns := append([]string{}, alwaysIgnore...)
	path := filepath.Join(cwd, ".outpostignore")
	f, err := os.Open(path)
	if err != nil {
		return patterns
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns
}

func shouldIgnoreRepo(rel string, patterns []string, isDir bool) bool {
	return shouldIgnore(rel, patterns, isDir)
}

// CollectRepoPathsForTest exposes repo path collection for tests.
func CollectRepoPathsForTest(cwd string) ([]string, error) {
	return collectRepoPaths(cwd)
}
