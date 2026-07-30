package config

import (
	"os"
	"path/filepath"
	"strings"
)

// ConfigDirEnv overrides the global Outpost config directory (~/.outpost) when set.
// Tests set this automatically; set OUTPOST_ALLOW_REAL_CONFIG=1 to opt out.
const ConfigDirEnv = "OUTPOST_CONFIG_DIR"

const allowRealConfigEnv = "OUTPOST_ALLOW_REAL_CONFIG"

func init() {
	isolateConfigForTests()
}

func isolateConfigForTests() {
	if os.Getenv(allowRealConfigEnv) == "1" {
		return
	}
	if strings.TrimSpace(os.Getenv(ConfigDirEnv)) != "" {
		return
	}
	if !runningGoTestBinary() {
		return
	}
	dir, err := os.MkdirTemp("", "outpost-test-config-*")
	if err != nil {
		panic("outpost: failed to isolate test config dir: " + err.Error())
	}
	_ = os.Setenv(ConfigDirEnv, dir)
}

func runningGoTestBinary() bool {
	return strings.Contains(filepath.Base(os.Args[0]), ".test")
}

// SetConfigDir overrides ~/.outpost for the current process.
func SetConfigDir(dir string) {
	_ = os.Setenv(ConfigDirEnv, dir)
}

// ConfigDirUnderHome returns the Outpost config path for a given home directory.
func ConfigDirUnderHome(home string) string {
	return filepath.Join(home, ".outpost")
}
