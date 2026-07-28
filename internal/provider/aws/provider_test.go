package aws_test

import (
	"testing"

	"github.com/degoke/outpost/internal/provider"
	"github.com/stretchr/testify/require"
)

func TestDefaultInstanceTypeConstant(t *testing.T) {
	require.Equal(t, "aws", provider.ProviderAWS)
}

func TestProviderStates(t *testing.T) {
	require.Equal(t, "running", provider.StateRunning)
	require.Equal(t, "stopped", provider.StateStopped)
}
