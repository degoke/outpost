package capabilities

import (
	"context"
	"strings"

	"github.com/goke/outpost/internal/inspect"
	"github.com/goke/outpost/internal/transport"
)

type Capability struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
}

type Report struct {
	Supported   []Capability `json:"supported"`
	Unavailable []Capability `json:"unavailable"`
}

func Detect(ctx context.Context, exec transport.Executor) (*Report, error) {
	checks := []struct {
		name string
		cmd  string
	}{
		{"docker", "docker info >/dev/null 2>&1"},
		{"compose", "docker compose version >/dev/null 2>&1"},
		{"kvm", "test -c /dev/kvm && grep -Eq 'vmx|svm' /proc/cpuinfo"},
		{"kind", "command -v kind >/dev/null 2>&1"},
		{"incus", "command -v incus >/dev/null 2>&1"},
		{"cgroup_v2", "stat -fc %T /sys/fs/cgroup/ 2>/dev/null | grep -q cgroup2fs"},
	}
	report := &Report{}
	for _, c := range checks {
		code, err := exec.Run(ctx, c.cmd, transport.RunOpts{})
		if err == nil && code == 0 {
			report.Supported = append(report.Supported, Capability{Name: c.name, Status: "available"})
		} else {
			reason := "not detected on host"
			if c.name == "kvm" {
				reason = "KVM device or CPU virtualization extensions unavailable"
			}
			report.Unavailable = append(report.Unavailable, Capability{Name: c.name, Status: "unavailable", Reason: reason})
		}
	}
	nested, err := inspect.RunOutput(ctx, exec, "curl -s --max-time 1 http://169.254.169.254/latest/meta-data/ 2>/dev/null | head -1 || true")
	if err == nil && strings.TrimSpace(nested) != "" {
		report.Supported = append(report.Supported, Capability{Name: "nested_virt", Status: "unknown", Reason: "cloud metadata present — check instance type for nested virtualization"})
	} else {
		report.Unavailable = append(report.Unavailable, Capability{Name: "nested_virt", Status: "unavailable", Reason: "no provider metadata (adopted host)"})
	}
	return report, nil
}
