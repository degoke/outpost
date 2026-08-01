package transport_test

import (
	"testing"

	"github.com/degoke/outpost/internal/transport"
	"github.com/stretchr/testify/require"
)

func TestStripChildTTY(t *testing.T) {
	require.Equal(t, []string{"run", "-i", "ubuntu"}, transport.StripChildTTY([]string{"run", "-it", "ubuntu"}))
	require.Equal(t, []string{"exec", "-i", "app", "bash"}, transport.StripChildTTY([]string{"exec", "-it", "app", "bash"}))
	require.Equal(t, []string{"run", "-i", "ubuntu"}, transport.StripChildTTY([]string{"run", "-i", "-t", "ubuntu"}))
}
