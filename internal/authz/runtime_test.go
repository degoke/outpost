package authz_test

import (
	"context"
	"testing"

	"github.com/goke/outpost/internal/authz"
	"github.com/goke/outpost/internal/config"
	"github.com/goke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
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
