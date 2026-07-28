package mirror

import (
	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/transport"
)

type Runner struct {
	Exec transport.Executor
	Proj *config.Project
	Cwd  string
	Host string
}

func New(exec transport.Executor, proj *config.Project, cwd, host string) *Runner {
	return &Runner{
		Exec: exec,
		Proj: proj,
		Cwd:  cwd,
		Host: host,
	}
}
