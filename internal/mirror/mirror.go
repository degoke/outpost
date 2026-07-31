package mirror

import (
	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/output"
	"github.com/degoke/outpost/internal/transport"
	"github.com/degoke/outpost/internal/upload"
)

type Runner struct {
	Exec          transport.Executor
	Proj          *config.Project
	Cwd           string
	Host          string
	Out           *output.Printer
	SyncUseRsync  bool
	SyncForceSFTP bool
	SyncWorkers   int
}

func New(exec transport.Executor, proj *config.Project, cwd, host string, out *output.Printer) *Runner {
	return &Runner{
		Exec: exec,
		Proj: proj,
		Cwd:  cwd,
		Host: host,
		Out:  out,
	}
}

func (r *Runner) syncOpts() *upload.SyncOpts {
	if r.Out == nil && !r.SyncUseRsync && r.SyncWorkers == 0 {
		return nil
	}
	return &upload.SyncOpts{
		Out:       r.Out,
		UseRsync:  r.SyncUseRsync,
		ForceSFTP: r.SyncForceSFTP,
		Workers:   r.SyncWorkers,
	}
}
