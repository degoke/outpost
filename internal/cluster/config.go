package cluster

import (
	"fmt"
	"strings"
)

type KindConfig struct {
	Name          string
	ControlPlanes int
	Workers       int
}

func RenderKindConfig(cfg KindConfig) string {
	if cfg.ControlPlanes == 0 {
		cfg.ControlPlanes = 1
	}
	var b strings.Builder
	b.WriteString("kind: Cluster\n")
	b.WriteString("apiVersion: kind.x-k8s.io/v1alpha4\n")
	b.WriteString(fmt.Sprintf("name: %s\n", cfg.Name))
	b.WriteString("nodes:\n")
	for i := 0; i < cfg.ControlPlanes; i++ {
		b.WriteString("- role: control-plane\n")
	}
	for i := 0; i < cfg.Workers; i++ {
		b.WriteString("- role: worker\n")
	}
	return b.String()
}

func EstimateResources(controlPlanes, workers int) (cpu float64, memBytes, diskBytes uint64) {
	if controlPlanes == 0 {
		controlPlanes = 1
	}
	cpu = float64(controlPlanes)*2 + float64(workers)
	memBytes = uint64(controlPlanes)*2*1024*1024*1024 + uint64(workers)*1024*1024*1024
	diskBytes = 5 * 1024 * 1024 * 1024
	return cpu, memBytes, diskBytes
}
