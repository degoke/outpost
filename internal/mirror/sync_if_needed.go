package mirror

import (
	"context"

	"github.com/degoke/outpost/internal/upload"
)

// SyncSkipReason explains why a sync was skipped automatically.
type SyncSkipReason string

const (
	SyncSkippedNone        SyncSkipReason = ""
	SyncSkippedWatchActive SyncSkipReason = "mirror watch is running"
	SyncSkippedUnchanged   SyncSkipReason = "no local changes since last sync"
)

func (r *Runner) syncIfNeeded(ctx context.Context, force bool) (SyncSkipReason, error) {
	if force {
		if err := r.syncAndRecord(ctx); err != nil {
			return SyncSkippedNone, err
		}
		return SyncSkippedNone, nil
	}
	if WatchActive(r.Host, r.Proj.Name, r.Cwd) {
		return SyncSkippedWatchActive, nil
	}
	needs, err := upload.NeedsRepoSync(r.Cwd, r.Host, r.Proj.Name)
	if err != nil {
		return SyncSkippedNone, err
	}
	if !needs {
		return SyncSkippedUnchanged, nil
	}
	if err := r.syncAndRecord(ctx); err != nil {
		return SyncSkippedNone, err
	}
	return SyncSkippedNone, nil
}

func (r *Runner) syncAndRecord(ctx context.Context) error {
	if r.Out != nil {
		r.Out.Step("Syncing repository...")
	}
	if err := r.Sync(ctx); err != nil {
		return err
	}
	return upload.MarkRepoSynced(r.Cwd, r.Host, r.Proj.Name)
}

func (r *Runner) recordSynced() error {
	return upload.MarkRepoSynced(r.Cwd, r.Host, r.Proj.Name)
}

func skipReasonMessage(reason SyncSkipReason) string {
	switch reason {
	case SyncSkippedWatchActive:
		return "Skipping sync (mirror watch is running)"
	case SyncSkippedUnchanged:
		return "Skipping sync (no local changes since last sync)"
	default:
		return ""
	}
}

func (r *Runner) logSyncSkip(reason SyncSkipReason) {
	if r.Out == nil || reason == "" {
		return
	}
	if msg := skipReasonMessage(reason); msg != "" {
		r.Out.Step(msg)
	}
}

// SyncExplicit performs a repository sync and records local state.
func (r *Runner) SyncExplicit(ctx context.Context) error {
	if err := r.Sync(ctx); err != nil {
		return err
	}
	return r.recordSynced()
}

// SyncIfNeeded syncs only when local files changed and watch is not already running.
func (r *Runner) SyncIfNeeded(ctx context.Context, force bool) (SyncSkipReason, error) {
	return r.syncIfNeeded(ctx, force)
}
