package mirror_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/mirror"
	"github.com/stretchr/testify/require"
)

func TestDetectPlanFromGoModAndMakefile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/foo\n\ngo 1.22\n\ntoolchain go1.22.5\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Makefile"), []byte("build:\n\tgo build ./...\n"), 0644))

	plan, err := mirror.DetectPlan(dir, nil, "make build")
	require.NoError(t, err)
	require.Equal(t, []string{"ca-certificates", "curl", "make"}, plan.Packages)
	require.Equal(t, "1.22.5", plan.GoVersion)
}

func TestDetectPlanFromMakefileWithoutGoMod(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Makefile"), []byte("build:\n\tgo build ./...\n"), 0644))

	plan, err := mirror.DetectPlan(dir, nil, "")
	require.NoError(t, err)
	require.Equal(t, []string{"ca-certificates", "curl", "make"}, plan.Packages)
	require.Equal(t, "1.22.5", plan.GoVersion)
}

func TestDetectPlanAddsCurlForExplicitGo(t *testing.T) {
	proj := &config.Project{
		Toolchain: &config.ProjectToolchain{Go: "1.23.0"},
	}
	plan, err := mirror.DetectPlan(t.TempDir(), proj, "")
	require.NoError(t, err)
	require.Equal(t, []string{"ca-certificates", "curl"}, plan.Packages)
	require.Equal(t, "1.23.0", plan.GoVersion)
}

func TestDetectPlanExplicitConfigOverridesGo(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n\ngo 1.21\n"), 0644))
	proj := &config.Project{
		Toolchain: &config.ProjectToolchain{
			Packages: []string{"git"},
			Go:       "1.23.0",
		},
	}
	plan, err := mirror.DetectPlan(dir, proj, "")
	require.NoError(t, err)
	require.Equal(t, []string{"ca-certificates", "curl", "git"}, plan.Packages)
	require.Equal(t, "1.23.0", plan.GoVersion)
}

func TestDetectPlanRejectsUnknownPackage(t *testing.T) {
	proj := &config.Project{
		Toolchain: &config.ProjectToolchain{Packages: []string{"nmap"}},
	}
	_, err := mirror.DetectPlan(t.TempDir(), proj, "")
	require.Error(t, err)
}

func TestWrapCommandPath(t *testing.T) {
	got := mirror.WrapCommandPathForTest("make build", []string{"/var/lib/outpost/toolchains/go/1.22.5/bin"})
	require.Equal(t, "export PATH=/var/lib/outpost/toolchains/go/1.22.5/bin:$PATH && make build", got)
}

func TestYumPackageMapping(t *testing.T) {
	require.Equal(t, []string{"gcc", "gcc-c++", "make"}, mirror.YumPackagesForTest([]string{"build-essential"}))
	script := mirror.PackageInstallScriptForTest([]string{"build-essential"})
	require.Contains(t, script, "gcc")
	require.Contains(t, script, "gcc-c++")
}
