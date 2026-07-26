package compose

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goke/outpost/internal/capacity"
	"github.com/goke/outpost/internal/config"
	"github.com/goke/outpost/internal/transport"
	"gopkg.in/yaml.v3"
)

func CheckComposeCapacity(ctx context.Context, exec transport.Executor, cwd string, proj *config.Project) error {
	var totalMem uint64
	var totalCPU float64
	for _, rel := range proj.ComposeFiles {
		path := filepath.Join(cwd, rel)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var doc map[string]any
		if err := yaml.Unmarshal(data, &doc); err != nil {
			continue
		}
		services, _ := doc["services"].(map[string]any)
		for _, svcVal := range services {
			svc, _ := svcVal.(map[string]any)
			deploy, _ := svc["deploy"].(map[string]any)
			resources, _ := deploy["resources"].(map[string]any)
			limits, _ := resources["limits"].(map[string]any)
			if limits == nil {
				continue
			}
			totalMem += parseLimitMemory(limits["memory"])
			totalCPU += parseLimitCPU(limits["cpus"])
		}
	}
	if totalMem == 0 && totalCPU == 0 {
		return nil
	}
	return capacity.Check(ctx, exec, capacity.Request{
		CPUCores: totalCPU, MemoryBytes: totalMem,
	})
}

func parseLimitCPU(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}

func parseLimitMemory(v any) uint64 {
	switch m := v.(type) {
	case string:
		return parseMemory(m)
	case int:
		return uint64(m)
	case int64:
		return uint64(m)
	case float64:
		return uint64(m)
	default:
		return 0
	}
}

func parseMemory(s string) uint64 {
	// supports 512m, 1g, etc.
	s = strings.ToLower(strings.TrimSpace(s))
	mult := uint64(1)
	switch {
	case strings.HasSuffix(s, "g"):
		mult = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "g")
	case strings.HasSuffix(s, "m"):
		mult = 1024 * 1024
		s = strings.TrimSuffix(s, "m")
	case strings.HasSuffix(s, "k"):
		mult = 1024
		s = strings.TrimSuffix(s, "k")
	}
	var v float64
	fmt.Sscanf(s, "%f", &v)
	return uint64(v * float64(mult))
}
