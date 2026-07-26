package project_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goke/outpost/internal/project"
	"github.com/stretchr/testify/require"
)

func TestInitStableName(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services: {}"), 0644))
	p1, err := project.Init(dir, "", "", false)
	require.NoError(t, err)
	require.Equal(t, filepath.Base(dir), p1.Name)

	p2, err := project.Init(dir, "", "", false)
	require.NoError(t, err)
	require.Equal(t, p1.Name, p2.Name)
	require.Equal(t, p1.RemoteDir, p2.RemoteDir)
}

func TestDeriveNameExplicit(t *testing.T) {
	require.Equal(t, "my-api", project.DeriveName("/tmp/foo", "My API"))
}
