package migrate_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/migrate"
	"github.com/degoke/outpost/internal/output"
	"github.com/degoke/outpost/internal/transport"
	"github.com/degoke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

func testExecutorFactory(global *config.Global) migrate.ExecutorFactory {
	return func(hostName string) (transport.Executor, *config.Host, error) {
		return mock.New(), global.Hosts[hostName], nil
	}
}

func TestRunRequiresDestinationHost(t *testing.T) {
	_, err := migrate.Run(context.Background(), migrate.Options{
		Project:     &config.Project{Name: "demo"},
		NewExecutor: func(hostName string) (transport.Executor, *config.Host, error) { return nil, nil, nil },
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--to is required")
}

func TestRunRejectsSameHost(t *testing.T) {
	global := &config.Global{
		ActiveHost: "dev",
		Hosts: map[string]*config.Host{
			"dev": {Hostname: "203.0.113.10", User: "ubuntu", Port: 22, Role: config.RoleOwner},
		},
	}
	_, err := migrate.Run(context.Background(), migrate.Options{
		Cwd:         t.TempDir(),
		Project:     &config.Project{Name: "demo", Host: "dev"},
		Global:      global,
		FromHost:    "dev",
		ToHost:      "dev",
		NewExecutor: testExecutorFactory(global),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "must differ")
}

func TestDryRunPrintsPlan(t *testing.T) {
	global := &config.Global{
		ActiveHost: "old",
		Hosts: map[string]*config.Host{
			"old": {Hostname: "203.0.113.10", User: "ubuntu", Port: 22, Role: config.RoleOwner},
			"new": {Hostname: "203.0.113.11", User: "ubuntu", Port: 22, Role: config.RoleOwner},
		},
	}
	cwd := t.TempDir()
	proj := &config.Project{
		Name:         "demo",
		Host:         "old",
		RemoteDir:    "/var/lib/outpost/projects/demo",
		ComposeFiles: []string{"docker-compose.yml"},
		Kubernetes:   &config.ProjectKubernetes{Driver: "kind"},
		Machine:      &config.ProjectMachine{Image: "ubuntu:24.04"},
		Environment:  &config.ProjectEnvironment{},
	}
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "docker-compose.yml"), []byte("services:\n  web:\n    image: nginx\nvolumes:\n  data:\n"), 0o644))

	result, err := migrate.Run(context.Background(), migrate.Options{
		Cwd:         cwd,
		Project:     proj,
		Global:      global,
		FromHost:    "old",
		ToHost:      "new",
		DryRun:      true,
		Out:         output.New(false, false),
		NewExecutor: testExecutorFactory(global),
	})
	require.NoError(t, err)
	require.Equal(t, "demo", result.Project)
	require.Equal(t, "old", result.FromHost)
	require.Equal(t, "new", result.ToHost)
}
