package environment

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDockerExecInteractiveUsesSingleTTY(t *testing.T) {
	cmd := dockerExecInteractive("outpost-dev-demo", "/bin/bash", "cd /workspace && exec /bin/bash")
	require.Contains(t, cmd, "docker exec -i")
	require.NotContains(t, cmd, "-it")
	require.Contains(t, cmd, "TERM=xterm-256color")
	require.Contains(t, cmd, "COLORTERM=truecolor")
}

func TestShellBootstrapDockerfileDoesNotRunUnpinnedInstaller(t *testing.T) {
	df := shellBootstrapDockerfile()
	require.NotContains(t, df, "starship.rs/install.sh")
	require.Contains(t, df, "command -v starship")
	require.Contains(t, df, "starship init bash")
}

func TestShellBootstrapScriptIsIdempotent(t *testing.T) {
	script := shellBootstrapScript()
	require.Contains(t, script, "command -v starship")
	require.Contains(t, script, "starship init bash")
}
