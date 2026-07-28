package project_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/degoke/outpost/internal/project"
	"github.com/stretchr/testify/require"
)

func TestInitStableName(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services: {}"), 0644))
	p1, err := project.Init(dir, "", "", false, false)
	require.NoError(t, err)
	require.Equal(t, filepath.Base(dir), p1.Name)

	p2, err := project.Init(dir, "", "", false, false)
	require.NoError(t, err)
	require.Equal(t, p1.Name, p2.Name)
	require.Equal(t, p1.RemoteDir, p2.RemoteDir)
}

func TestInitNoCompose(t *testing.T) {
	dir := t.TempDir()
	p, err := project.Init(dir, "scripts", "", false, true)
	require.NoError(t, err)
	require.Equal(t, "scripts", p.Name)
	require.Empty(t, p.ComposeFiles)
}

func TestInitPreservesPythonConfig(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".outpost"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".outpost", "project.yaml"), []byte(`version: 1
name: scripts
remote_dir: /var/lib/outpost/projects/scripts
compose_files: []
python:
  venv: .venv
  requirements: requirements.txt
`), 0644))

	p, err := project.Init(dir, "scripts", "", false, true)
	require.NoError(t, err)
	require.NotNil(t, p.Python)
	require.Equal(t, ".venv", p.Python.Venv)
	require.Equal(t, "requirements.txt", p.Python.Requirements)
}

func TestDeriveNameExplicit(t *testing.T) {
	require.Equal(t, "my-api", project.DeriveName("/tmp/foo", "My API"))
}
