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
