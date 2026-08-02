package authz_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/degoke/outpost/internal/authz"
	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v3"
)

func TestRequireRuntimeAccessApproved(t *testing.T) {
	exec := mock.New()
	manifest := config.ShareManifest{
		Version: 1,
		Devices: []config.Device{{
			ID:     "dev-1",
			Label:  "laptop",
			Status: config.DeviceApproved,
		}},
	}
	data, err := yaml.Marshal(manifest)
	require.NoError(t, err)
	exec.Files[config.ShareManifestPath] = data

	h := &config.Host{Role: config.RoleMember, DeviceID: "dev-1"}
	require.NoError(t, authz.RequireRuntimeAccess(context.Background(), h, exec))
}

func TestRequireRuntimeAccessDetectsTamperedOwnerRole(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	keyPEM, err := ssh.MarshalPrivateKey(priv, "")
	require.NoError(t, err)
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "device.key")
	require.NoError(t, os.WriteFile(keyPath, pem.EncodeToMemory(keyPEM), 0600))
	sshPub, err := ssh.NewPublicKey(pub)
	require.NoError(t, err)
	pubLine := string(ssh.MarshalAuthorizedKey(sshPub))

	exec := mock.New()
	manifest := config.ShareManifest{Version: 1, Devices: []config.Device{{
		ID: "device-1", PublicKey: pubLine, Status: config.DeviceApproved,
	}}}
	data, err := yaml.Marshal(manifest)
	require.NoError(t, err)
	exec.Files[config.ShareManifestPath] = data

	h := &config.Host{Role: config.RoleOwner, IdentityFile: keyPath}
	require.NoError(t, authz.RequireRuntimeAccess(context.Background(), h, exec))
	require.Equal(t, config.RoleMember, h.Role)
	require.Equal(t, "device-1", h.DeviceID)
	require.Error(t, authz.RequireOwner(h, "invite approve"))
}

func TestRequireRuntimeAccessPending(t *testing.T) {
	exec := mock.New()
	manifest := config.ShareManifest{
		Version: 1,
		Devices: []config.Device{{
			ID:     "dev-1",
			Label:  "laptop",
			Status: config.DevicePending,
		}},
	}
	data, err := yaml.Marshal(manifest)
	require.NoError(t, err)
	exec.Files[config.ShareManifestPath] = data

	h := &config.Host{Role: config.RoleMember, DeviceID: "dev-1"}
	require.Error(t, authz.RequireRuntimeAccess(context.Background(), h, exec))
}
