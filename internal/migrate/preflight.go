package migrate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/connect"
	"github.com/degoke/outpost/internal/environment"
	"github.com/degoke/outpost/internal/inspect"
	"github.com/degoke/outpost/internal/mirror"
	"github.com/degoke/outpost/internal/output"
	"github.com/degoke/outpost/internal/transport"
)

func checkMigratePreflight(host, project string) error {
	return connect.EnsureNoActiveSession(host, project)
}

func checkRemoteSessions(ctx context.Context, exec transport.Executor, proj *config.Project) error {
	prefix := "outpost-" + proj.Name + "-"
	cmd := fmt.Sprintf("tmux list-sessions -F '#{session_name}' 2>/dev/null || true")
	raw, err := inspect.RunOutput(ctx, exec, cmd)
	if err != nil {
		return err
	}
	var active []string
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, prefix) {
			continue
		}
		if short, ok := mirror.ShortSessionName(proj, line); ok {
			active = append(active, short)
		}
	}
	if len(active) == 0 {
		return nil
	}
	return fmt.Errorf("remote mirror sessions still running on source host — stop them first: %s", strings.Join(active, ", "))
}

func ensureSourceDevContainer(ctx context.Context, exec transport.Executor, cwd string, proj *config.Project, out *output.Printer) error {
	if !proj.EnvironmentEnabled() {
		return nil
	}
	if out != nil {
		out.Step("Ensuring development container exists on source before export...")
	}
	return environment.New(exec, proj, cwd).Ensure(ctx)
}

func appContainerWarning(cwd string, proj *config.Project, ctx context.Context, exec transport.Executor) string {
	if !fileExists(filepath.Join(cwd, "Dockerfile")) {
		return ""
	}
	name := "outpost-app-" + config.SanitizeProjectName(proj.Name)
	cmd := fmt.Sprintf("docker ps -aq --filter name=^/%s$ 2>/dev/null", name)
	out, err := inspect.RunOutput(ctx, exec, cmd)
	if err != nil {
		return ""
	}
	if strings.TrimSpace(out) != "" {
		return ""
	}
	return fmt.Sprintf("Dockerfile present but application container %q was not found on the source host — run 'outpost app run --detach' before migrate if you need it migrated", name)
}

func partialFailureHint(result *Result, projectName string, err error) error {
	if result == nil || (!result.DockerExported && !result.MachineExported && !result.RemoteStateMoved) {
		return err
	}
	dir, dirErr := config.VolumeArchivesDir(projectName)
	if dirErr != nil {
		return fmt.Errorf("%w\n\nDestination import failed after source export; local archives were kept for retry (project host was not changed)", err)
	}
	if _, statErr := os.Stat(dir); statErr != nil {
		return fmt.Errorf("%w\n\nDestination import failed after source export; local archives were kept for retry (project host was not changed)", err)
	}
	return fmt.Errorf("%w\n\nDestination import failed after source export. Local archives are in %s — fix the error and re-run migrate to retry import (project host was not changed).", err, dir)
}

// PartialFailureHintForTest exposes partial failure messaging for tests.
func PartialFailureHintForTest(result *Result, projectName string, err error) error {
	return partialFailureHint(result, projectName, err)
}
