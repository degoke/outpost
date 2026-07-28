package disk

import (
	"context"
	"fmt"

	"github.com/degoke/outpost/internal/inspect"
	"github.com/degoke/outpost/internal/transport"
)

type Report struct {
	Filesystem  FilesystemUsage       `json:"filesystem"`
	Docker      inspect.DockerSummary `json:"docker"`
	Outpost     inspect.OutpostDirs   `json:"outpost"`
	Reclaimable ReclaimableSummary    `json:"reclaimable"`
}

type FilesystemUsage struct {
	TotalBytes     uint64  `json:"total_bytes"`
	UsedBytes      uint64  `json:"used_bytes"`
	AvailableBytes uint64  `json:"available_bytes"`
	UsedPercent    float64 `json:"used_percent"`
}

type ReclaimableSummary struct {
	StoppedContainers int    `json:"stopped_containers"`
	DanglingImages    int    `json:"dangling_images"`
	UnusedNetworks    int    `json:"unused_networks"`
	BuildCache        string `json:"build_cache,omitempty"`
	UploadArtifacts   string `json:"upload_artifacts,omitempty"`
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
	outpost, _ := inspect.CollectOutpostDirs(ctx, exec)
	reclaimable := estimateReclaimable(ctx, exec, docker)
	usedPct := 0.0
	if host.DiskTotal > 0 {
		usedPct = float64(host.DiskUsed) / float64(host.DiskTotal) * 100
	}
	return &Report{
		Filesystem: FilesystemUsage{
			TotalBytes: host.DiskTotal, UsedBytes: host.DiskUsed,
			AvailableBytes: host.DiskAvailable, UsedPercent: usedPct,
		},
		Docker: docker, Outpost: outpost, Reclaimable: reclaimable,
	}, nil
}

func estimateReclaimable(ctx context.Context, exec transport.Executor, docker inspect.DockerSummary) ReclaimableSummary {
	var r ReclaimableSummary
	r.StoppedContainers = docker.ContainersStop
	for _, row := range docker.DiskUsage {
		switch row.Type {
		case "Images":
			if row.Total > row.Active {
				r.DanglingImages = int(row.Total - row.Active)
			}
		case "Build Cache":
			r.BuildCache = row.Reclaimable
		}
	}
	if n, err := inspect.CountUnusedNetworks(ctx, exec); err == nil {
		r.UnusedNetworks = n
	}
	if files, err := inspect.ListUploadTempFiles(ctx, exec); err == nil && len(files) > 0 {
		r.UploadArtifacts = fmt.Sprintf("%d file(s)", len(files))
	}
	return r
}
