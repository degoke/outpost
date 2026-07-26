package capabilities

import (
	"context"
	"strings"

	"github.com/goke/outpost/internal/config"
	"github.com/goke/outpost/internal/inspect"
	"github.com/goke/outpost/internal/transport"
)

type Capability struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type Report struct {
	Supported   []Capability `json:"supported"`
	Unavailable []Capability `json:"unavailable"`
}

func Detect(ctx context.Context, exec transport.Executor) (*Report, error) {
	return DetectWithProvider(ctx, exec, nil)
}

func DetectWithProvider(ctx context.Context, exec transport.Executor, providerMeta *config.ProviderMeta) (*Report, error) {
	checks := []struct {
		name string
		cmd  string
	}{
		{"docker", "docker info >/dev/null 2>&1"},
		{"compose", "docker compose version >/dev/null 2>&1"},
		{"kind", "command -v kind >/dev/null 2>&1"},
		{"kubectl", "command -v kubectl >/dev/null 2>&1"},
		{"incus", "command -v incus >/dev/null 2>&1"},
		{"cgroup_v2", "stat -fc %T /sys/fs/cgroup/ 2>/dev/null | grep -q cgroup2fs"},
	}
	report := &Report{}
	for _, c := range checks {
		if c.name == "kvm" {
			if state, reason := checkKVM(ctx, exec); state == "supported" {
				report.Supported = append(report.Supported, Capability{Name: "kvm", Status: "available"})
			} else {
				report.Unavailable = append(report.Unavailable, Capability{Name: "kvm", Status: "unavailable", Reason: reason})
			}
			continue
		}
		code, err := exec.Run(ctx, c.cmd, transport.RunOpts{})
		if err == nil && code == 0 {
			report.Supported = append(report.Supported, Capability{Name: c.name, Status: "available"})
		} else {
			reason := "not detected on host"
			report.Unavailable = append(report.Unavailable, Capability{Name: c.name, Status: "unavailable", Reason: reason})
		}
	}
	instanceType := resolveInstanceType(ctx, exec, providerMeta)
	if instanceType != "" {
		if nestedVirtCapableInstanceType(instanceType) {
			report.Supported = append(report.Supported, Capability{
				Name: "nested_virt", Status: "available",
				Reason: "instance type " + instanceType + " supports nested virtualization",
			})
		} else {
			report.Unavailable = append(report.Unavailable, Capability{
				Name: "nested_virt", Status: "unavailable",
				Reason: "instance type " + instanceType + " does not expose KVM for nested virtualization",
			})
		}
	} else {
		nested, err := inspect.RunOutput(ctx, exec, "curl -s --max-time 1 http://169.254.169.254/latest/meta-data/ 2>/dev/null | head -1 || true")
		if err == nil && strings.TrimSpace(nested) != "" {
			report.Supported = append(report.Supported, Capability{Name: "nested_virt", Status: "unknown", Reason: "cloud metadata present — check instance type for nested virtualization"})
		} else {
			report.Unavailable = append(report.Unavailable, Capability{Name: "nested_virt", Status: "unavailable", Reason: "no provider metadata (adopted host)"})
		}
	}
	vmReport, err := VMSupport(ctx, exec, providerMeta)
	if err == nil {
		vmCap := vmCapabilityFromReport(vmReport)
		if vmCap.Status == "available" {
			report.Supported = append(report.Supported, vmCap)
		} else {
			report.Unavailable = append(report.Unavailable, vmCap)
		}
	}
	return report, nil
}
