package mirror_test

import (
	"testing"

	"github.com/goke/outpost/internal/config"
	"github.com/goke/outpost/internal/mirror"
	"github.com/stretchr/testify/require"
)

func TestTmuxSessionName(t *testing.T) {
	proj := &config.Project{Name: "demo"}
	require.Equal(t, "outpost-demo-gen", mirror.TmuxSessionName(proj, "gen"))
	short, ok := mirror.ShortSessionName(proj, "outpost-demo-gen")
	require.True(t, ok)
	require.Equal(t, "gen", short)
}

func TestSanitizeSessionName(t *testing.T) {
	name, err := mirror.SanitizeSessionName("gen batch 1")
	require.NoError(t, err)
	require.Equal(t, "gen-batch-1", name)
}
