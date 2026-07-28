package mirror_test

import (
	"testing"

	"github.com/degoke/outpost/internal/mirror"
	"github.com/stretchr/testify/require"
)

func TestDetachedInnerCommandPreservesQuotes(t *testing.T) {
	original := `node scripts/gen.js --name "batch '1'"`
	logPath := "/var/lib/outpost/projects/demo/.outpost/sessions/gen.log"
	inner := mirror.DetachedInnerCommandForTest(original, logPath)
	require.Contains(t, inner, "base64 -d")
	require.Contains(t, inner, "EXIT:$?")
	require.Contains(t, inner, logPath)

	decoded, err := mirror.DecodeDetachedCommandForTest(inner)
	require.NoError(t, err)
	require.Equal(t, original, decoded)
}
