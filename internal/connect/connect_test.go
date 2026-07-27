package connect_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/goke/outpost/internal/config"
	"github.com/goke/outpost/internal/connect"
	"github.com/goke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

func TestParseComposePorts(t *testing.T) {
	dir := t.TempDir()
	compose := `services:
  web:
    ports:
      - "8080:80"
      - "127.0.0.1:3000:3000"
  api:
    ports:
      - 9000
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(compose), 0644))
	proj := &config.Project{ComposeFiles: []string{"docker-compose.yml"}}
	mappings, err := connect.ParseComposePorts(dir, proj, "")
	require.NoError(t, err)
	require.Len(t, mappings, 3)
	var web8080 *connect.PortMapping
	for i := range mappings {
		if mappings[i].Service == "web" && mappings[i].HostPort == 8080 {
			web8080 = &mappings[i]
			break
		}
	}
	require.NotNil(t, web8080)
	require.Equal(t, 80, web8080.TargetPort)
}

func TestParseManualPort(t *testing.T) {
	pm, err := connect.ParseManualPort("9090:80")
	require.NoError(t, err)
	require.Equal(t, 9090, pm.HostPort)
	require.Equal(t, 80, pm.TargetPort)
}

func TestStartForwardsUsesHostPortOnRemote(t *testing.T) {
	exec := mock.New()
	mappings := []connect.PortMapping{{
		Service: "web", HostPort: 8080, TargetPort: 80, BindHost: "127.0.0.1",
	}}
	_, closers, err := connect.StartForwards(context.Background(), exec, mappings, nil)
	require.NoError(t, err)
	for _, c := range closers {
		c.Close()
	}
}

func TestCheckLocalPortConflict(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	_, portStr, err := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, err)
	port := 0
	_, err = fmt.Sscanf(portStr, "%d", &port)
	require.NoError(t, err)
	err = connect.CheckLocalPort("127.0.0.1", port)
	require.Error(t, err)
	require.Contains(t, err.Error(), "already in use")
}

func TestEnsureNoActiveSessionStale(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	host, project := "personal", "demo"
	sess := &connect.Session{
		Host: host, Project: project, PID: 999999999,
	}
	require.NoError(t, connect.SaveSession(sess))
	require.NoError(t, connect.EnsureNoActiveSession(host, project))
	_, err := connect.LoadSession(host, project)
	require.Error(t, err)
}

func TestEnsureNoActiveSessionBlocksLive(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	host, project := "personal", "demo"
	sess := &connect.Session{
		Host: host, Project: project, PID: os.Getpid(),
	}
	require.NoError(t, connect.SaveSession(sess))
	err := connect.EnsureNoActiveSession(host, project)
	require.Error(t, err)
	require.Contains(t, err.Error(), "already active")
}
