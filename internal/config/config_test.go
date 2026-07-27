package config_test

import (
	"os"
	"path/filepath"
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

func TestResetLocal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dir, err := config.ConfigDir()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "identities", "host-1"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("version: 1\n"), 0600))

	require.NoError(t, config.ResetLocal())

	_, err = os.Stat(dir)
	require.True(t, os.IsNotExist(err))

	require.NoError(t, config.ResetLocal())
}
