package transport_test

import (
	"errors"
	"testing"

	"github.com/degoke/outpost/internal/transport"
	"github.com/stretchr/testify/require"
)

func TestExitStatus(t *testing.T) {
	code, ok := transport.ExitStatus(&transport.ExitError{Code: 42})
	require.True(t, ok)
	require.Equal(t, 42, code)

	_, ok = transport.ExitStatus(errors.New("boom"))
	require.False(t, ok)
}
