package share_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/share"
	"github.com/degoke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestInvitationWorkflow(t *testing.T) {
	exec := mock.New()
	host := &config.Host{Name: "personal", Role: config.RoleOwner, HostID: "host-123"}
	global := &config.Global{Hosts: map[string]*config.Host{"personal": host}}
	svc := &share.Service{Global: global, Exec: exec, Host: host}

	ctx := context.Background()
	code, err := svc.CreateInvitation(ctx, time.Hour)
	require.NoError(t, err)
	require.NotEmpty(t, code)

	data, err := exec.Download(config.ShareManifestPath)
	require.NoError(t, err)
	var m config.ShareManifest
	require.NoError(t, yaml.Unmarshal(data, &m))
	require.Len(t, m.Invitations, 1)

	err = svc.JoinInvitation(ctx, code, "laptop", "203.0.113.1", "ubuntu", 22)
	require.NoError(t, err)

	manifest, err := svc.List(ctx)
	require.NoError(t, err)
	require.Len(t, manifest.Devices, 1)
	require.Equal(t, config.DevicePending, manifest.Devices[0].Status)
	deviceID := manifest.Devices[0].ID
	require.NotEmpty(t, global.Hosts)
	for _, h := range global.Hosts {
		if h.Role == config.RoleMember {
			require.Equal(t, deviceID, h.DeviceID)
		}
	}

	require.NoError(t, svc.Approve(ctx, deviceID))
	manifest, err = svc.List(ctx)
	require.NoError(t, err)
	require.Equal(t, config.DeviceApproved, manifest.FindDevice(deviceID).Status)

	keys, err := exec.Download(config.ShareAuthorizedKeysPath)
	require.NoError(t, err)
	require.Contains(t, string(keys), "ssh-ed25519")

	require.NoError(t, svc.Revoke(ctx, deviceID))
	keys, err = exec.Download(config.ShareAuthorizedKeysPath)
	require.NoError(t, err)
	require.Equal(t, "", strings.TrimSpace(string(keys)))
}
