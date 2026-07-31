package cleanup_test

import (
	"context"
	"testing"

	"github.com/degoke/outpost/internal/cleanup"
	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

func TestProjectCleanupRemovesManagedArtifactsWithRetention(t *testing.T) {
	exec := mock.New()
	project := &config.Project{
		Name:      "demo",
		RemoteDir: "/var/lib/outpost/projects/demo",
		Cleanup:   &config.ProjectCleanup{LogRetentionDays: 2, BuildCacheDays: 5, StoppedContainerDays: 1},
	}
	require.NoError(t, cleanup.Project(context.Background(), exec, project, cleanup.OptionsForProject(project)))
	require.True(t, exec.HasCommand("-type f -name '*.log' -mtime +2"))
	require.True(t, exec.HasCommand("docker container prune -f --filter label=com.outpost.managed=true --filter until=24h"))
	require.True(t, exec.HasCommand("docker builder prune -f --filter until=120h"))
}
