package upload

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/degoke/outpost/internal/config"
)

const (
	OutpostIgnoreFile    = ".outpostignore"
	OutpostIgnoreRelPath = ".outpost/.outpostignore"
)

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
	patterns := loadOutpostIgnore(cwd)
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		rel := filepath.ToSlash(line)
		if shouldIgnoreRepo(rel, patterns, false) {
			continue
		}
		paths = append(paths, rel)
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

// LoadOutpostIgnorePatterns returns built-in and .outpostignore patterns.
func LoadOutpostIgnorePatterns(cwd string) []string {
	return loadOutpostIgnore(cwd)
}

func loadOutpostIgnore(cwd string) []string {
	patterns := append([]string{}, alwaysIgnore...)
	patterns = append(patterns, readIgnorePatterns(config.OutpostIgnorePath(cwd))...)
	patterns = append(patterns, readIgnorePatterns(config.LegacyOutpostIgnorePath(cwd))...)
	return patterns
}

func readIgnorePatterns(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var patterns []string
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

// IsIgnoredByOutpost reports whether rel matches built-in or .outpostignore rules.
func IsIgnoredByOutpost(cwd, rel string) bool {
	patterns := loadOutpostIgnore(cwd)
	return shouldIgnoreRepo(filepath.ToSlash(filepath.Clean(rel)), patterns, false)
}

// IsSyncableRepoPath reports whether rel should be included in repository sync.
func IsSyncableRepoPath(cwd, rel string) bool {
	rel = filepath.ToSlash(filepath.Clean(rel))
	if rel == "." || rel == "" {
		return false
	}
	if IsIgnoredByOutpost(cwd, rel) {
		return false
	}
	if isGitRepo(cwd) {
		return !gitIgnoredByGit(cwd, rel)
	}
	return true
}

func gitIgnoredByGit(cwd, rel string) bool {
	cmd := exec.Command("git", "check-ignore", "-q", "--", rel)
	cmd.Dir = cwd
	err := cmd.Run()
	if err == nil {
		return true
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false
	}
	return false
}

// OutpostIgnoreExcludeArg returns a repo-relative path for rsync --exclude-from, if configured.
func OutpostIgnoreExcludeArg(cwd string) (string, bool) {
	if _, err := os.Stat(config.OutpostIgnorePath(cwd)); err == nil {
		return OutpostIgnoreRelPath, true
	}
	if _, err := os.Stat(config.LegacyOutpostIgnorePath(cwd)); err == nil {
		return OutpostIgnoreFile, true
	}
	return "", false
}

// HasOutpostIgnoreFile reports whether an outpost ignore file exists.
func HasOutpostIgnoreFile(cwd string) bool {
	_, ok := OutpostIgnoreExcludeArg(cwd)
	return ok
}

// CollectRepoPathsForTest exposes repo path collection for tests.
func CollectRepoPathsForTest(cwd string) ([]string, error) {
	return collectRepoPaths(cwd)
}
