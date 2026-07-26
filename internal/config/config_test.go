package config_test

import (
	"testing"

	"github.com/goke/outpost/internal/config"
	"github.com/stretchr/testify/require"
)

func TestSanitizeProjectName(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"My App", "my-app"},
		{"  foo_bar  ", "foo-bar"},
		{"***", "project"},
		{"api-v2", "api-v2"},
	}
	for _, tc := range tests {
		require.Equal(t, tc.want, config.SanitizeProjectName(tc.in))
	}
}

func TestGlobalResolveHost(t *testing.T) {
	g := &config.Global{
		ActiveHost: "a",
		Hosts: map[string]*config.Host{
			"a": {Hostname: "1.2.3.4", User: "ubuntu", Port: 22},
		},
	}
	h, err := g.ResolveHost("")
	require.NoError(t, err)
	require.Equal(t, "a", h.Name)

	_, err = g.ResolveHost("missing")
	require.Error(t, err)
}
