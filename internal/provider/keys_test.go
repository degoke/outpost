package provider_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/degoke/outpost/internal/provider"
	"github.com/stretchr/testify/require"
)

func TestEnsureProvisionKeyIdempotent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	pub1, priv1, err := provider.EnsureProvisionKey("host-test")
	require.NoError(t, err)
	require.Contains(t, pub1, "ssh-ed25519")
	pub2, priv2, err := provider.EnsureProvisionKey("host-test")
	require.NoError(t, err)
	require.Equal(t, pub1, pub2)
	require.Equal(t, priv1, priv2)
	info, err := os.Stat(priv1)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestProvisionKeyPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, priv, err := provider.EnsureProvisionKey("abc")
	require.NoError(t, err)
	got, err := provider.ProvisionKeyPath("abc")
	require.NoError(t, err)
	require.Equal(t, priv, got)
	_, err = provider.ProvisionKeyPath("missing")
	require.Error(t, err)
	_ = filepath.Base(priv)
}
