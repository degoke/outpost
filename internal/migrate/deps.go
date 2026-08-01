package migrate

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/degoke/outpost/internal/config"
)

func dependencyVolumes(cwd string, proj *config.Project) []DockerVolume {
	if proj.Environment != nil && len(proj.Environment.Volumes) > 0 {
		return environmentVolumes(proj)
	}
	prefix := "outpost-deps-" + config.SanitizeProjectName(proj.Name)
	var out []DockerVolume
	if fileExists(filepath.Join(cwd, "package.json")) {
		out = append(out, DockerVolume{ArchiveName: "deps-node.tar.gz", DockerName: prefix + "-node"})
	}
	if fileExists(filepath.Join(cwd, "requirements.txt")) || fileExists(filepath.Join(cwd, "pyproject.toml")) {
		out = append(out, DockerVolume{ArchiveName: "deps-python.tar.gz", DockerName: prefix + "-python"})
	}
	if fileExists(filepath.Join(cwd, "go.mod")) {
		out = append(out, DockerVolume{ArchiveName: "deps-go.tar.gz", DockerName: prefix + "-go"})
	}
	return out
}

func environmentVolumes(proj *config.Project) []DockerVolume {
	if proj.Environment == nil {
		return nil
	}
	var out []DockerVolume
	for _, mount := range proj.Environment.Volumes {
		name := strings.TrimSpace(mount.Name)
		if name == "" {
			continue
		}
		out = append(out, DockerVolume{
			ArchiveName: "env-" + config.SanitizeProjectName(name) + ".tar.gz",
			DockerName:  name,
		})
	}
	return out
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// DependencyVolumesForTest exposes dependency volume discovery for tests.
func DependencyVolumesForTest(cwd string, proj *config.Project) []DockerVolume {
	return dependencyVolumes(cwd, proj)
}
