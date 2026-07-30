package compose

import (
	"github.com/degoke/outpost/internal/mirror"
	"github.com/degoke/outpost/internal/upload"
)

func (r *Runner) syncProjectIfNeeded() error {
	if mirror.WatchActive(r.HostName, r.Project.Name, r.Cwd) {
		if r.Out != nil {
			r.Out.Step("Skipping sync (mirror watch is running)")
		}
		return nil
	}
	needs, err := upload.NeedsProjectSync(r.Cwd, r.Project, r.HostName)
	if err != nil {
		return err
	}
	if !needs {
		if r.Out != nil {
			r.Out.Step("Skipping sync (no local changes since last sync)")
		}
		return nil
	}
	if r.Out != nil {
		r.Out.Step("Syncing project files...")
	}
	opts := (*upload.SyncOpts)(nil)
	if r.Out != nil {
		opts = &upload.SyncOpts{Out: r.Out}
	}
	if err := upload.SyncProject(r.Cwd, r.Project, r.Exec, opts); err != nil {
		return err
	}
	return upload.MarkProjectSynced(r.Cwd, r.Project, r.HostName)
}
