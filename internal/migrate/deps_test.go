package migrate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/migrate"
	"github.com/stretchr/testify/require"
)

func TestDependencyVolumesDetectsNodeModules(t *testing.T) {
	cwd := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "package.json"), []byte("{}"), 0o644))
	proj := &config.Project{Name: "demo", Environment: &config.ProjectEnvironment{}}
	vols := migrate.DependencyVolumesForTest(cwd, proj)
	require.Len(t, vols, 1)
	require.Equal(t, "outpost-deps-demo-node", vols[0].DockerName)
}

func TestProjectDockerVolumesIncludesComposeAndDeps(t *testing.T) {
	cwd := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "package.json"), []byte("{}"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "docker-compose.yml"), []byte("services:\n  web:\n    image: nginx\nvolumes:\n  data:\n"), 0o644))
	proj := &config.Project{
		Name:         "demo",
		ComposeFiles: []string{"docker-compose.yml"},
		Environment:  &config.ProjectEnvironment{},
	}
	vols := migrate.ProjectDockerVolumesForTest(cwd, proj)
	names := map[string]bool{}
	for _, v := range vols {
		names[v.DockerName] = true
	}
	require.True(t, names["outpost-deps-demo-node"])
	require.True(t, names["demo_data"])
}

func TestDockerBundleExistsForTest(t *testing.T) {
	require.False(t, migrate.DockerBundleExistsForTest("missing"))
	dir, err := config.VolumeArchivesDir("demo")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docker-bundle.tar"), []byte("x"), 0o600))
	require.True(t, migrate.DockerBundleExistsForTest("demo"))
}
