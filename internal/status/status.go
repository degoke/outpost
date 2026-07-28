package status

import (
	"context"

	"github.com/degoke/outpost/internal/cluster"
	"github.com/degoke/outpost/internal/inspect"
	"github.com/degoke/outpost/internal/machine"
	"github.com/degoke/outpost/internal/share"
	"github.com/degoke/outpost/internal/transport"
)

type Report struct {
	Host     inspect.HostMetrics      `json:"host"`
	Docker   inspect.DockerSummary    `json:"docker"`
	Compose  []inspect.ComposeProject `json:"compose_projects,omitempty"`
	Sharing  SharingSummary           `json:"sharing"`
	Clusters int                      `json:"clusters"`
	Machines int                      `json:"machines"`
}

type SharingSummary struct {
	ApprovedDevices int `json:"approved_devices"`
}

func Collect(ctx context.Context, exec transport.Executor) (*Report, error) {
	host, err := inspect.CollectHostMetrics(ctx, exec)
	if err != nil {
		return nil, err
	}
	docker, err := inspect.CollectDockerSummary(ctx, exec)
	if err != nil {
		return nil, err
	}
	compose, _ := inspect.ListComposeProjects(ctx, exec)
	approved, _ := share.ApprovedCount(ctx, exec)
	clusters, _ := cluster.Count(ctx, exec)
	machines, _ := machine.Count(ctx, exec)
	return &Report{
		Host:     host,
		Docker:   docker,
		Compose:  compose,
		Sharing:  SharingSummary{ApprovedDevices: approved},
		Clusters: clusters,
		Machines: machines,
	}, nil
}
