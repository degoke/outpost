package upload

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/degoke/outpost/internal/config"
	"gopkg.in/yaml.v3"
)

var alwaysIgnore = []string{
	".git",
	".outpost",
	".DS_Store",
}

// AlwaysIgnorePatterns returns built-in ignore patterns used during sync.
func AlwaysIgnorePatterns() []string {
	return append([]string{}, alwaysIgnore...)
}

// ShouldIgnoreRel reports whether a repository-relative path should be skipped.
func ShouldIgnoreRel(rel string, patterns []string, isDir bool) bool {
	return shouldIgnore(rel, patterns, isDir)
}

func collectSyncPaths(cwd string, proj *config.Project) ([]string, error) {
	seen := map[string]bool{}
	var paths []string
	add := func(rel string) error {
		rel = filepath.ToSlash(filepath.Clean(rel))
		if rel == "." || rel == "" {
			return nil
		}
		if seen[rel] {
			return nil
		}
		local := filepath.Join(cwd, rel)
		info, err := os.Stat(local)
		if err != nil {
			return fmt.Errorf("local file %s: %w", rel, err)
		}
		if info.IsDir() {
			return fmt.Errorf("expected file, got directory %s", rel)
		}
		seen[rel] = true
		paths = append(paths, rel)
		return nil
	}

	for _, rel := range allComposeFiles(proj) {
		if err := add(rel); err != nil {
			return nil, err
		}
	}
	if _, err := os.Stat(filepath.Join(cwd, ".env")); err == nil {
		if err := add(".env"); err != nil {
			return nil, err
		}
	}

	contexts, err := parseBuildContexts(cwd, allComposeFiles(proj))
	if err != nil {
		return nil, err
	}
	for _, ctx := range contexts {
		contextFiles, err := walkBuildContext(cwd, ctx)
		if err != nil {
			return nil, err
		}
		for _, rel := range contextFiles {
			if err := add(rel); err != nil {
				return nil, err
			}
		}
	}

	return paths, nil
}

func parseBuildContexts(cwd string, composeFiles []string) ([]string, error) {
	seen := map[string]bool{}
	var contexts []string
	add := func(ctx string) {
		ctx = filepath.ToSlash(filepath.Clean(ctx))
		if ctx == "" {
			ctx = "."
		}
		if seen[ctx] {
			return
		}
		seen[ctx] = true
		contexts = append(contexts, ctx)
	}

	for _, rel := range composeFiles {
		data, err := os.ReadFile(filepath.Join(cwd, rel))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", rel, err)
		}
		var doc map[string]any
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("parse %s: %w", rel, err)
		}
		services, _ := doc["services"].(map[string]any)
		for _, raw := range services {
			svc, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			build, ok := svc["build"]
			if !ok {
				continue
			}
			switch b := build.(type) {
			case string:
				add(b)
			case map[string]any:
				ctx := "."
				if c, ok := b["context"].(string); ok && strings.TrimSpace(c) != "" {
					ctx = c
				}
				add(ctx)
			}
		}
	}
	return contexts, nil
}

func walkBuildContext(cwd, contextDir string) ([]string, error) {
	root := filepath.Join(cwd, contextDir)
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("build context %s: %w", contextDir, err)
	}
	if !info.IsDir() {
		return []string{filepath.ToSlash(filepath.Clean(contextDir))}, nil
	}

	ignorePatterns := loadIgnorePatterns(root)
	var files []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(cwd, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if shouldIgnore(rel, ignorePatterns, true) {
				return filepath.SkipDir
			}
			return nil
		}
		if shouldIgnore(rel, ignorePatterns, false) {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	return files, err
}

func loadIgnorePatterns(contextRoot string) []string {
	patterns := append([]string{}, alwaysIgnore...)
	path := filepath.Join(contextRoot, ".dockerignore")
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

func shouldIgnore(rel string, patterns []string, isDir bool) bool {
	rel = filepath.ToSlash(rel)
	base := filepath.Base(rel)
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if strings.HasSuffix(pattern, "/") {
			dir := strings.TrimSuffix(pattern, "/")
			if rel == dir || strings.HasPrefix(rel, dir+"/") {
				return true
			}
			continue
		}
		if matched, _ := filepath.Match(pattern, base); matched {
			return true
		}
		if matched, _ := filepath.Match(pattern, rel); matched {
			return true
		}
		if rel == pattern || strings.HasPrefix(rel, pattern+"/") {
			return true
		}
	}
	if isDir && (rel == ".git" || rel == ".outpost" || strings.HasSuffix(rel, "/node_modules")) {
		return true
	}
	return false
}

func remotePath(proj *config.Project, rel string) string {
	return proj.RemoteDir + "/" + filepath.ToSlash(rel)
}

// CollectSyncPathsForTest exposes sync path collection for tests.
func CollectSyncPathsForTest(cwd string, proj *config.Project) ([]string, error) {
	return collectSyncPaths(cwd, proj)
}
