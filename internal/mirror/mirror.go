package mirror

import (
	"github.com/goke/outpost/internal/config"
	"github.com/goke/outpost/internal/transport"
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
