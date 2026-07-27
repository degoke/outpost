package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goke/outpost/internal/config"
	"gopkg.in/yaml.v3"
)

var composeCandidates = []string{
	"docker-compose.yml",
	"docker-compose.yaml",
	"compose.yml",
	"compose.yaml",
}

var overrideCandidates = []string{
	"docker-compose.override.yml",
	"docker-compose.override.yaml",
	"compose.override.yml",
	"compose.override.yaml",
}

func DetectComposeFile(cwd string) (string, error) {
	for _, name := range composeCandidates {
		path := filepath.Join(cwd, name)
		if _, err := os.Stat(path); err == nil {
			return name, nil
		}
	}
	return "", fmt.Errorf("no compose file found in %s (expected docker-compose.yml or compose.yaml)", cwd)
}

func DeriveName(cwd, explicit string) string {
	if explicit != "" {
		return config.SanitizeProjectName(explicit)
	}
	return config.SanitizeProjectName(filepath.Base(cwd))
}

func Init(cwd, name, host string, writeGitignore, noCompose bool) (*config.Project, error) {
	var composeFiles []string
	if !noCompose {
		composeFile, err := DetectComposeFile(cwd)
		if err != nil {
			return nil, err
		}
		composeFiles = []string{composeFile}
	}
	projectName := DeriveName(cwd, name)

	existingPath := config.ProjectConfigPath(cwd)
	var existing *config.Project
	if data, err := os.ReadFile(existingPath); err == nil {
		var p config.Project
		if yamlErr := yaml.Unmarshal(data, &p); yamlErr == nil {
			existing = &p
		}
	}

	p := &config.Project{
		Name:         projectName,
		Host:         host,
		RemoteDir:    filepath.Join(config.DefaultRemoteBase, "projects", projectName),
		ComposeFiles: composeFiles,
	}

	if existing != nil {
		if existing.Name != projectName {
			return nil, fmt.Errorf("project already initialized as %q — passing --name %q would rename it; delete .outpost/project.yaml first if intentional", existing.Name, projectName)
		}
		p.Name = existing.Name
		p.RemoteDir = existing.RemoteDir
		p.ExtraFiles = existing.ExtraFiles
		p.Python = existing.Python
		if host == "" {
			p.Host = existing.Host
		}
		if noCompose {
			p.ComposeFiles = nil
		} else if len(composeFiles) > 0 {
			p.ComposeFiles = composeFiles
		} else if len(existing.ComposeFiles) > 0 {
			p.ComposeFiles = existing.ComposeFiles
		}
	}

	extra, err := detectExtraFiles(cwd, p.ExtraFiles)
	if err != nil {
		return nil, err
	}
	p.ExtraFiles = extra

	if err := config.SaveProject(cwd, p); err != nil {
		return nil, err
	}

	if writeGitignore {
		if err := appendGitignore(cwd); err != nil {
			return nil, err
		}
	}

	return p, nil
}

func detectExtraFiles(cwd string, existing []string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	add := func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		out = append(out, name)
	}
	for _, f := range existing {
		add(f)
	}
	for _, name := range overrideCandidates {
		if _, err := os.Stat(filepath.Join(cwd, name)); err == nil {
			add(name)
		}
	}
	return out, nil
}

func appendGitignore(cwd string) error {
	path := filepath.Join(cwd, ".gitignore")
	const entry = ".outpost/\n"
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if strings.Contains(string(data), ".outpost") {
		return nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(entry)
	return err
}

func ResolveHostName(globalHost, projectHost string) string {
	if projectHost != "" {
		return projectHost
	}
	return globalHost
}
