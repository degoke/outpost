package testenv

import (
	"testing"

	"github.com/degoke/outpost/internal/config"
)

// UseConfigDir redirects ~/.outpost to dir for the duration of a test.
func UseConfigDir(t *testing.T, dir string) {
	t.Helper()
	t.Setenv(config.ConfigDirEnv, dir)
}

// UseHomeConfigDir redirects ~/.outpost to $HOME/.outpost for the test.
func UseHomeConfigDir(t *testing.T, home string) string {
	t.Helper()
	dir := config.ConfigDirUnderHome(home)
	UseConfigDir(t, dir)
	return dir
}
