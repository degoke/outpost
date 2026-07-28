package integration_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/degoke/outpost/internal/authz"
	"github.com/degoke/outpost/internal/compose"
	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/connect"
	"github.com/degoke/outpost/internal/share"
	"github.com/degoke/outpost/internal/transport"
	"github.com/degoke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestComposeUpTwiceSameProject(t *testing.T) {
	exec := mock.New()
	cwd := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "docker-compose.yml"), []byte("services:\n  web:\n    image: nginx\n"), 0644))
	proj := &config.Project{
		Name:         "demo",
		RemoteDir:    "/var/lib/outpost/projects/demo",
		ComposeFiles: []string{"docker-compose.yml"},
	}
	runner := &compose.Runner{Exec: exec, Project: proj, Cwd: cwd, HostName: "personal", ForceYes: true}

	ctx := context.Background()
	_, err := runner.Run(ctx, "up", []string{"-d"}, false)
	require.NoError(t, err)
	first := exec.LastCommand()
	_, err = runner.Run(ctx, "up", []string{"-d"}, false)
	require.NoError(t, err)
	second := exec.LastCommand()
	require.Equal(t, first, second)
	require.True(t, strings.Contains(first, "-p 'demo'"))
}

func TestMockSSHFailure(t *testing.T) {
	exec := mock.New()
	exec.Responses["false"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{ExitCode: 1, Stderr: "permission denied"}
	code, err := exec.Run(context.Background(), "false", transport.RunOpts{})
	require.NoError(t, err)
	require.Equal(t, 1, code)
}

func TestInviteApproveConnectRevokeFlow(t *testing.T) {
	exec := mock.New()
	ownerHost := &config.Host{Name: "personal", Role: config.RoleOwner, HostID: "host-123"}
	global := &config.Global{Hosts: map[string]*config.Host{"personal": ownerHost}}
	svc := &share.Service{Global: global, Exec: exec, Host: ownerHost}
	ctx := context.Background()

	code, err := svc.CreateInvitation(ctx, time.Hour)
	require.NoError(t, err)

	err = svc.JoinInvitation(ctx, code, "laptop", "203.0.113.1", "ubuntu", 22)
	require.NoError(t, err)

	manifest, err := svc.List(ctx)
	require.NoError(t, err)
	deviceID := manifest.Devices[0].ID
	require.NoError(t, svc.Approve(ctx, deviceID))

	var memberHost *config.Host
	for _, h := range global.Hosts {
		if h.Role == config.RoleMember {
			memberHost = h
			break
		}
	}
	require.NotNil(t, memberHost)
	require.NoError(t, authz.RequireRuntimeAccess(ctx, memberHost, exec))

	cwd := t.TempDir()
	composeFile := `services:
  web:
    ports:
      - "8080:80"
`
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "docker-compose.yml"), []byte(composeFile), 0644))
	proj := &config.Project{Name: "demo", ComposeFiles: []string{"docker-compose.yml"}}
	mappings, err := connect.ParseComposePorts(cwd, proj, "")
	require.NoError(t, err)
	require.Len(t, mappings, 1)

	_, closers, err := connect.StartForwards(ctx, exec, mappings, nil)
	require.NoError(t, err)
	for _, c := range closers {
		c.Close()
	}

	require.NoError(t, svc.Revoke(ctx, deviceID))
	require.Error(t, authz.RequireRuntimeAccess(ctx, memberHost, exec))

	keys, err := exec.Download(config.ShareAuthorizedKeysPath)
	require.NoError(t, err)
	require.Equal(t, "", strings.TrimSpace(string(keys)))
}

func TestRevokedDeviceInManifest(t *testing.T) {
	exec := mock.New()
	manifest := config.ShareManifest{
		Version: 1,
		Devices: []config.Device{{
			ID: "dev-1", Label: "laptop", Status: config.DeviceRevoked,
		}},
	}
	data, err := yaml.Marshal(manifest)
	require.NoError(t, err)
	exec.Files[config.ShareManifestPath] = data

	h := &config.Host{Role: config.RoleMember, DeviceID: "dev-1"}
	require.Error(t, authz.RequireRuntimeAccess(context.Background(), h, exec))
}
