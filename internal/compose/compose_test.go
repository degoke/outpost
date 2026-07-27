package compose_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goke/outpost/internal/compose"
	"github.com/goke/outpost/internal/config"
	"github.com/goke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

func TestBuildCommandStableProject(t *testing.T) {
	r := &compose.Runner{
		Project: &config.Project{
			Name:         "myapp",
			RemoteDir:    "/var/lib/outpost/projects/myapp",
			ComposeFiles: []string{"docker-compose.yml"},
			ExtraFiles:   []string{"docker-compose.override.yml"},
		},
	}
	cmd := r.BuildCommand("up", []string{"-d"})
	require.Contains(t, cmd, "-p 'myapp'")
	require.Contains(t, cmd, "-f /var/lib/outpost/projects/myapp/docker-compose.yml")
	require.Contains(t, cmd, "-f /var/lib/outpost/projects/myapp/docker-compose.override.yml")
	require.Contains(t, cmd, " up -d")
}

func TestIsDestructive(t *testing.T) {
	require.True(t, compose.IsDestructive("down", []string{}))
	require.True(t, compose.IsDestructive("down", []string{"-v"}))
	require.False(t, compose.IsDestructive("up", []string{"-d"}))
}

func TestParseNamedVolumes(t *testing.T) {
	dir := t.TempDir()
	composeFile := filepath.Join(dir, "docker-compose.yml")
	require.NoError(t, os.WriteFile(composeFile, []byte(`
services:
  db:
    image: postgres
    volumes:
      - postgres_data:/var/lib/postgresql/data
volumes:
  postgres_data:
  external_data:
    external: true
  shared:
    external:
      name: existing
  custom:
    name: shared_custom_volume
`), 0644))
	override := filepath.Join(dir, "docker-compose.override.yml")
	require.NoError(t, os.WriteFile(override, []byte(`
volumes:
  cache_data:
`), 0644))

	proj := &config.Project{
		Name:         "my-api",
		ComposeFiles: []string{"docker-compose.yml"},
		ExtraFiles:   []string{"docker-compose.override.yml"},
	}
	vols, err := compose.ParseNamedVolumes(dir, proj)
	require.NoError(t, err)
	require.Len(t, vols, 3)

	names := map[string]string{}
	for _, v := range vols {
		names[v.LogicalName] = v.DockerName
	}
	require.Equal(t, "my-api_postgres_data", names["postgres_data"])
	require.Equal(t, "my-api_cache_data", names["cache_data"])
	require.Equal(t, "shared_custom_volume", names["custom"])
	require.NotContains(t, names, "external_data")
	require.NotContains(t, names, "shared")
}

func TestResolveVolumesFromComposeConfig(t *testing.T) {
	exec := mock.New()
	exec.Responses["docker compose -p 'my-api' -f /var/lib/outpost/projects/my-api/docker-compose.yml config --format json"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{
		Stdout: `{"volumes":{"postgres_data":{"name":"my_api_postgres_data"},"external_data":{"external":true}}}`,
	}
	proj := &config.Project{
		Name:         "my-api",
		RemoteDir:    "/var/lib/outpost/projects/my-api",
		ComposeFiles: []string{"docker-compose.yml"},
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(`
volumes:
  postgres_data:
  external_data:
    external: true
`), 0644))

	vols, err := compose.ResolveVolumes(t.Context(), exec, dir, proj)
	require.NoError(t, err)
	require.Len(t, vols, 1)
	require.Equal(t, "postgres_data", vols[0].LogicalName)
	require.Equal(t, "my_api_postgres_data", vols[0].DockerName)
}

func TestExportAndImportVolumes(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	composeFile := filepath.Join(dir, "docker-compose.yml")
	require.NoError(t, os.WriteFile(composeFile, []byte(`
services:
  db:
    image: postgres
volumes:
  postgres_data:
`), 0644))

	proj := &config.Project{
		Name:         "my-api",
		RemoteDir:    "/var/lib/outpost/projects/my-api",
		ComposeFiles: []string{"docker-compose.yml"},
	}
	exec := mock.New()
	mockVolumePresent(exec, "my-api_postgres_data")
	exec.Files["/var/lib/outpost/projects/my-api/.volume-staging/postgres_data.tar.gz"] = []byte("archive")

	require.NoError(t, compose.ExportVolumes(t.Context(), exec, dir, proj, "personal", compose.VolumeOptions{}))

	archiveDir, err := config.VolumeArchivesDir("my-api")
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(archiveDir, "postgres_data.tar.gz"))
	require.NoError(t, err)
	require.Equal(t, "personal", proj.Volumes.LastHost)

	exec2 := mock.New()
	mockVolumeMissing(exec2, "my-api_postgres_data")
	exec2.Responses["docker volume create 'my-api_postgres_data'"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{ExitCode: 0}
	exec2.Responses["docker run --rm -v 'my-api_postgres_data':/to -v /var/lib/outpost/projects/my-api/.volume-staging:/from:ro alpine tar xzf /from/postgres_data.tar.gz -C /to"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{ExitCode: 0}

	require.NoError(t, compose.ImportVolumes(t.Context(), exec2, dir, proj, "staging", compose.VolumeOptions{}))
	require.True(t, exec2.HasCommand("docker volume create 'my-api_postgres_data'"))
}

func TestEnsureImportedSkipsNonemptyVolumes(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(`
volumes:
  postgres_data:
`), 0644))
	proj := &config.Project{
		Name:         "my-api",
		RemoteDir:    "/var/lib/outpost/projects/my-api",
		ComposeFiles: []string{"docker-compose.yml"},
	}
	archiveDir, err := config.VolumeArchivesDir("my-api")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(archiveDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(archiveDir, "postgres_data.tar.gz"), []byte("archive"), 0600))

	exec := mock.New()
	mockVolumePresent(exec, "my-api_postgres_data")

	require.NoError(t, compose.EnsureImported(t.Context(), exec, dir, proj, "staging", true))
	require.False(t, exec.HasCommand("docker volume create 'my-api_postgres_data'"))
}

func TestEnsureImportedRestoresEmptyVolume(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(`
volumes:
  postgres_data:
`), 0644))
	proj := &config.Project{
		Name:         "my-api",
		RemoteDir:    "/var/lib/outpost/projects/my-api",
		ComposeFiles: []string{"docker-compose.yml"},
		Volumes:      &config.ProjectVolumeState{LastHost: "personal"},
	}
	archiveDir, err := config.VolumeArchivesDir("my-api")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(archiveDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(archiveDir, "postgres_data.tar.gz"), []byte("archive"), 0600))

	exec := mock.New()
	mockVolumePresentEmpty(exec, "my-api_postgres_data")
	exec.Responses["docker volume rm 'my-api_postgres_data'"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{ExitCode: 0}
	exec.Responses["docker volume create 'my-api_postgres_data'"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{ExitCode: 0}
	exec.Responses["docker run --rm -v 'my-api_postgres_data':/to -v /var/lib/outpost/projects/my-api/.volume-staging:/from:ro alpine tar xzf /from/postgres_data.tar.gz -C /to"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{ExitCode: 0}

	require.NoError(t, compose.EnsureImported(t.Context(), exec, dir, proj, "staging", true))
	require.True(t, exec.HasCommand("docker volume create 'my-api_postgres_data'"))
}

func mockVolumePresent(exec *mock.Executor, name string) {
	exec.Responses["docker volume inspect '"+name+"' >/dev/null 2>&1"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{ExitCode: 0}
	exec.Responses["docker run --rm -v '"+name+"':/data:ro alpine sh -c 'if [ -z \"$(ls -A /data 2>/dev/null)\" ]; then exit 0; else exit 1; fi'"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{ExitCode: 1}
}

func mockVolumePresentEmpty(exec *mock.Executor, name string) {
	exec.Responses["docker volume inspect '"+name+"' >/dev/null 2>&1"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{ExitCode: 0}
	exec.Responses["docker run --rm -v '"+name+"':/data:ro alpine sh -c 'if [ -z \"$(ls -A /data 2>/dev/null)\" ]; then exit 0; else exit 1; fi'"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{ExitCode: 0}
}

func mockVolumeMissing(exec *mock.Executor, name string) {
	exec.Responses["docker volume inspect '"+name+"' >/dev/null 2>&1"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{ExitCode: 1}
}
