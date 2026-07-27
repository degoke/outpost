package capacity

import (
	"context"
	"fmt"
	"math"

	"github.com/goke/outpost/internal/inspect"
	"github.com/goke/outpost/internal/transport"
)

const safetyMargin = 0.10

type Request struct {
	CPUCores    float64
	MemoryBytes uint64
	DiskBytes   uint64
}

type Report struct {
	Host           inspect.HostMetrics `json:"host"`
	UsedMemory     uint64              `json:"used_memory_bytes"`
	UsedCPU        float64             `json:"used_cpu_cores"`
	AvailableMem   uint64              `json:"available_memory_bytes"`
	AvailableCPU   float64             `json:"available_cpu_cores"`
	AvailableDisk  uint64              `json:"available_disk_bytes"`
	Recommendation string              `json:"recommendation"`
}

func Collect(ctx context.Context, exec transport.Executor) (*Report, error) {
	host, err := inspect.CollectHostMetrics(ctx, exec)
	if err != nil {
		return nil, err
	}
	stats, _ := inspect.ListContainerStats(ctx, exec)
	var usedMem uint64
	var usedCPU float64
	for _, s := range stats {
		usedMem += s.MemUsage
		usedCPU += s.CPUPercent / 100.0
	}
	availMem := availableAfterMargin(host.MemoryAvailable, host.MemoryTotal)
	availCPU := math.Max(0, float64(host.CPUCores)*(1-safetyMargin)-usedCPU)
	availDisk := availableAfterMargin(host.DiskAvailable, host.DiskTotal)
	rec := fmt.Sprintf("Can fit ~%d more 1 CPU / 1 GiB workloads", int(math.Min(availCPU, float64(availMem)/(1024*1024*1024))))
	return &Report{
		Host: host, UsedMemory: usedMem, UsedCPU: usedCPU,
		AvailableMem: availMem, AvailableCPU: availCPU, AvailableDisk: availDisk,
		Recommendation: rec,
	}, nil
}

func Check(ctx context.Context, exec transport.Executor, req Request) error {
	rep, err := Collect(ctx, exec)
	if err != nil {
		return err
	}
	return CheckWithReport(rep, req)
}

func CheckWithReport(rep *Report, req Request) error {
	if req.MemoryBytes > 0 && req.MemoryBytes > rep.AvailableMem {
		return fmt.Errorf("insufficient memory: requested %s, available %s (host has %s, %s in use)",
			formatBytes(req.MemoryBytes), formatBytes(rep.AvailableMem),
			formatBytes(rep.Host.MemoryTotal), formatBytes(rep.Host.MemoryUsed))
	}
	if req.CPUCores > 0 && req.CPUCores > rep.AvailableCPU {
		return fmt.Errorf("insufficient CPU: requested %.1f cores, available %.1f (host has %d cores, %.1f in use)",
			req.CPUCores, rep.AvailableCPU, rep.Host.CPUCores, rep.UsedCPU)
	}
	if req.DiskBytes > 0 && req.DiskBytes > rep.AvailableDisk {
		return fmt.Errorf("insufficient disk: requested %s, available %s (host has %s, %s in use)",
			formatBytes(req.DiskBytes), formatBytes(rep.AvailableDisk),
			formatBytes(rep.Host.DiskTotal), formatBytes(rep.Host.DiskUsed))
	}
	return nil
}

// AvailableAfterMargin subtracts a safety margin from available resources.
func AvailableAfterMargin(available, total uint64) uint64 {
	return availableAfterMargin(available, total)
}

func availableAfterMargin(available, total uint64) uint64 {
	if total == 0 {
		return available
	}
	margin := uint64(float64(total) * safetyMargin)
	if available > margin {
		return available - margin
	}
	return 0
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}
