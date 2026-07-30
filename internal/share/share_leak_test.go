package share_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/share"
	"github.com/degoke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

func TestJoinInvitationDoesNotWriteRealHomeConfig(t *testing.T) {
	realHome, err := os.UserHomeDir()
	require.NoError(t, err)
	realConfig := filepath.Join(realHome, ".outpost", "config.yaml")
	before, err := os.ReadFile(realConfig)
	if err != nil && !os.IsNotExist(err) {
		require.NoError(t, err)
	}

	exec := mock.New()
	host := &config.Host{Name: "personal", Role: config.RoleOwner, HostID: "host-123"}
	global := &config.Global{Hosts: map[string]*config.Host{"personal": host}}
	svc := &share.Service{Global: global, Exec: exec, Host: host}

	ctx := context.Background()
	code, err := svc.CreateInvitation(ctx, time.Hour)
	require.NoError(t, err)
	require.NoError(t, svc.JoinInvitation(ctx, code, "laptop", "203.0.113.1", "ubuntu", 22))

	after, err := os.ReadFile(realConfig)
	if os.IsNotExist(err) {
		require.Nil(t, before)
		return
	}
	require.NoError(t, err)
	require.Equal(t, string(before), string(after))
}
