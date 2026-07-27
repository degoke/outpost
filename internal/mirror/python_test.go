package mirror_test

import (
	"testing"

	"github.com/goke/outpost/internal/mirror"
	"github.com/stretchr/testify/require"
)

func TestRewritePythonCommand(t *testing.T) {
	require.Equal(t, ".venv/bin/python scripts/gen.py",
		mirror.RewritePythonCommand(true, ".venv", "python scripts/gen.py", false))
	require.Equal(t, ".venv/bin/python scripts/gen.py",
		mirror.RewritePythonCommand(true, ".venv", "python3 scripts/gen.py", false))
	require.Equal(t, "python scripts/gen.py",
		mirror.RewritePythonCommand(false, ".venv", "python scripts/gen.py", false))
	require.Equal(t, "python scripts/gen.py",
		mirror.RewritePythonCommand(true, ".venv", "python scripts/gen.py", true))
	require.Equal(t, ".venv/bin/python scripts/gen.py",
		mirror.RewritePythonCommand(true, ".venv", ".venv/bin/python scripts/gen.py", false))
	require.Equal(t, "node script.js",
		mirror.RewritePythonCommand(true, ".venv", "node script.js", false))
}
