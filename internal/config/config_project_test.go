package config_test

import (
	"testing"

	"github.com/degoke/outpost/internal/config"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestProjectPythonYAML(t *testing.T) {
	data := []byte(`
version: 1
name: demo
remote_dir: /var/lib/outpost/projects/demo
compose_files: []
python:
  venv: .venv
  requirements: requirements-dev.txt
`)
	var p config.Project
	require.NoError(t, yaml.Unmarshal(data, &p))
	require.NotNil(t, p.Python)
	require.Equal(t, ".venv", p.Python.Venv)
	require.Equal(t, "requirements-dev.txt", p.Python.Requirements)
}

func TestProjectRequireCompose(t *testing.T) {
	require.Error(t, (&config.Project{}).RequireCompose())
	require.NoError(t, (&config.Project{ComposeFiles: []string{"docker-compose.yml"}}).RequireCompose())
}

func TestProjectToolchainYAML(t *testing.T) {
	data := []byte(`
version: 1
name: demo
remote_dir: /var/lib/outpost/projects/demo
compose_files: []
toolchain:
  packages: [make, git]
  go: "1.22.5"
  auto: true
`)
	var p config.Project
	require.NoError(t, yaml.Unmarshal(data, &p))
	require.NotNil(t, p.Toolchain)
	require.Equal(t, []string{"make", "git"}, p.Toolchain.Packages)
	require.Equal(t, "1.22.5", p.Toolchain.Go)
	require.True(t, p.ToolchainAuto())
}

func TestProjectToolchainAutoDisabled(t *testing.T) {
	auto := false
	p := &config.Project{Toolchain: &config.ProjectToolchain{Auto: &auto}}
	require.False(t, p.ToolchainAuto())
}

func TestProjectKubernetesYAML(t *testing.T) {
	data := []byte(`
version: 1
name: demo
remote_dir: /var/lib/outpost/projects/demo
compose_files: []
kubernetes:
  driver: k3d
`)
	var p config.Project
	require.NoError(t, yaml.Unmarshal(data, &p))
	require.NotNil(t, p.Kubernetes)
	require.Equal(t, "k3d", p.Kubernetes.Driver)
}

func TestProjectAIYAML(t *testing.T) {
	data := []byte(`
version: 1
name: demo
remote_dir: /var/lib/outpost/projects/demo
compose_files: []
ai:
  command: claude
`)
	var p config.Project
	require.NoError(t, yaml.Unmarshal(data, &p))
	require.NotNil(t, p.AI)
	require.Equal(t, "claude", p.AI.Command)
}
