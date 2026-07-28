package transport_test

import (
	"errors"
	"testing"

	"github.com/degoke/outpost/internal/transport"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh/knownhosts"
)

func TestIsUnknownHostKey(t *testing.T) {
	require.True(t, transport.IsUnknownHostKeyForTest(errors.New("knownhosts: key is unknown")))
	require.True(t, transport.IsUnknownHostKeyForTest(errors.New(`knownhosts: "1.2.3.4" is not in known_hosts`)))
	require.True(t, transport.IsUnknownHostKeyForTest(&knownhosts.KeyError{}))
	require.False(t, transport.IsUnknownHostKeyForTest(&knownhosts.KeyError{Want: []knownhosts.KnownKey{{}}}))
}
