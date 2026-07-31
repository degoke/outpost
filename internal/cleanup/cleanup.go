package cleanup

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/transport"
)

type Options struct {
	LogRetentionDays     int
	BuildCacheDays       int
	StoppedContainerDays int
	IncludeDockerCache   bool
	MaxProjectGiB        int
}

func OptionsForProject(project *config.Project) Options {
	o := Options{LogRetentionDays: 7, BuildCacheDays: 14, StoppedContainerDays: 3, IncludeDockerCache: true}
	if project == nil || project.Cleanup == nil {
		return o
	}
	if project.Cleanup.LogRetentionDays > 0 {
		o.LogRetentionDays = project.Cleanup.LogRetentionDays
	}
	if project.Cleanup.BuildCacheDays > 0 {
		o.BuildCacheDays = project.Cleanup.BuildCacheDays
	}
	if project.Cleanup.StoppedContainerDays > 0 {
		o.StoppedContainerDays = project.Cleanup.StoppedContainerDays
	}
	o.IncludeDockerCache = project.CleanupEnabled()
	o.MaxProjectGiB = project.Cleanup.MaxProjectGiB
	return o
}

// Project removes only artifacts owned by Outpost or the selected project.
func Project(ctx context.Context, exec transport.Executor, project *config.Project, opts Options) error {
	if project == nil || !project.CleanupEnabled() {
		return nil
	}
	if opts.LogRetentionDays < 1 {
		opts.LogRetentionDays = 7
	}
	if opts.BuildCacheDays < 1 {
		opts.BuildCacheDays = 14
	}
	if opts.StoppedContainerDays < 1 {
		opts.StoppedContainerDays = 3
	}
	commands := []string{
		fmt.Sprintf("find %s/.outpost/sessions -type f -name '*.log' -mtime +%s -delete 2>/dev/null || true", quote(project.RemoteDir), strconv.Itoa(opts.LogRetentionDays)),
		fmt.Sprintf("find %s/.volume-staging -type f -mtime +1 -delete 2>/dev/null || true", quote(project.RemoteDir)),
		fmt.Sprintf("docker container prune -f --filter label=com.outpost.managed=true --filter until=%sh 2>/dev/null || true", strconv.Itoa(opts.StoppedContainerDays*24)),
	}
	if opts.IncludeDockerCache {
		commands = append(commands, fmt.Sprintf("docker builder prune -f --filter until=%dh 2>/dev/null || true", opts.BuildCacheDays*24))
	}
	for _, cmd := range commands {
		if _, err := exec.Run(ctx, cmd, transport.RunOpts{}); err != nil {
			return err
		}
	}
	if opts.MaxProjectGiB > 0 {
		var out bytes.Buffer
		if _, err := exec.Run(ctx, fmt.Sprintf("du -sk %s 2>/dev/null || true", quote(project.RemoteDir)), transport.RunOpts{Stdout: &out}); err != nil {
			return err
		}
		var kib int64
		if _, err := fmt.Sscanf(strings.TrimSpace(out.String()), "%d", &kib); err == nil && kib > int64(opts.MaxProjectGiB)*1024*1024 {
			return fmt.Errorf("project %q exceeds its %d GiB storage limit", project.Name, opts.MaxProjectGiB)
		}
	}
	return nil
}

// Global removes old Outpost-only caches that are not tied to a live project.
// It is intentionally exposed through the explicit cleanup command, not every
// shell startup, because it scans shared host state.
func Global(ctx context.Context, exec transport.Executor, opts Options) error {
	if opts.BuildCacheDays < 1 {
		opts.BuildCacheDays = 14
	}
	commands := []string{
		"find /var/lib/outpost/projects -path '*/.upload-tmp/*' -type f -mtime +1 -delete 2>/dev/null || true",
		"find /var/lib/outpost/toolchains -mindepth 2 -maxdepth 2 -type d -mtime +30 -exec sh -c 'for d; do v=$(basename \"$d\"); if ! grep -R -q \"$v\" /var/lib/outpost/projects/*/.outpost/toolchain.json 2>/dev/null; then rm -rf \"$d\"; fi; done' sh {} + 2>/dev/null || true",
		fmt.Sprintf("docker builder prune -f --filter until=%dh 2>/dev/null || true", opts.BuildCacheDays*24),
	}
	for _, cmd := range commands {
		if _, err := exec.Run(ctx, cmd, transport.RunOpts{}); err != nil {
			return err
		}
	}
	return nil
}

func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
