package capabilities

import (
	"context"
	"strings"

	"github.com/goke/outpost/internal/config"
	"github.com/goke/outpost/internal/inspect"
	"github.com/goke/outpost/internal/transport"
)

type VMSupportReport struct {
	Status      string `json:"status"`
	Reason      string `json:"reason"`
	Remediation string `json:"remediation,omitempty"`
}

func (r *VMSupportReport) CanCreateVM() bool {
	return r.Status == "supported"
}

func VMSupport(ctx context.Context, exec transport.Executor, providerMeta *config.ProviderMeta) (*VMSupportReport, error) {
	incusOK := commandOK(ctx, exec, "command -v incus >/dev/null 2>&1")
	kvmState, kvmReason := checkKVM(ctx, exec)

	if kvmState == "supported" && incusOK {
		return &VMSupportReport{
			Status: "supported",
			Reason: "KVM and Incus are available for hardware-virtualized machines",
		}, nil
	}

	instanceType := resolveInstanceType(ctx, exec, providerMeta)
	if kvmState != "supported" && instanceType != "" {
		reason := kvmReason
		if reason == "" {
			reason = "KVM is not available on this cloud instance"
		}
		if !nestedVirtCapableInstanceType(instanceType) {
			reason = "nested virtualization is not available on instance type " + instanceType
		}
		return &VMSupportReport{
			Status: "nested_possible",
			Reason: reason,
			Remediation: "Use a system container instead: outpost machine create NAME --image ubuntu:24.04\n" +
				"For VMs on AWS, choose a bare-metal instance type (for example *.metal) or enable nested virtualization on a supported instance type.\n" +
				"On bare metal or a home server, verify KVM with: outpost host capabilities",
		}, nil
	}

	reason := kvmReason
	if reason == "" {
		reason = "KVM device or CPU virtualization extensions are unavailable"
	}
	if !incusOK {
		reason += "; Incus is not installed"
	}
	return &VMSupportReport{
		Status: "unsupported",
		Reason: reason,
		Remediation: "Create a lightweight system container instead: outpost machine create NAME --image ubuntu:24.04\n" +
			"VMs require a host with KVM support and Incus installed.",
	}, nil
}

func checkKVM(ctx context.Context, exec transport.Executor) (status, reason string) {
	code, _ := exec.Run(ctx, "test -c /dev/kvm", transport.RunOpts{})
	if code != 0 {
		return "unsupported", "KVM device (/dev/kvm) is not present"
	}
	code, _ = exec.Run(ctx, "test -r /dev/kvm", transport.RunOpts{})
	if code != 0 {
		return "unsupported", "KVM device exists but is not readable by the current user"
	}
	code, _ = exec.Run(ctx, "grep -Eq 'vmx|svm' /proc/cpuinfo", transport.RunOpts{})
	if code != 0 {
		return "unsupported", "CPU virtualization extensions (vmx/svm) were not detected"
	}
	return "supported", ""
}

func commandOK(ctx context.Context, exec transport.Executor, cmd string) bool {
	code, err := exec.Run(ctx, cmd, transport.RunOpts{})
	return err == nil && code == 0
}

func resolveInstanceType(ctx context.Context, exec transport.Executor, providerMeta *config.ProviderMeta) string {
	if providerMeta != nil && providerMeta.InstanceType != "" {
		return providerMeta.InstanceType
	}
	out, err := inspect.RunOutput(ctx, exec, "curl -s --max-time 1 http://169.254.169.254/latest/meta-data/instance-type 2>/dev/null || true")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func nestedVirtCapableInstanceType(instanceType string) bool {
	instanceType = strings.ToLower(strings.TrimSpace(instanceType))
	if instanceType == "" {
		return false
	}
	if strings.Contains(instanceType, "metal") {
		return true
	}
	// AWS bare-metal and some compute-optimized families may support nested virt when enabled.
	prefixes := []string{"c5.", "m5.", "r5.", "i3."}
	for _, p := range prefixes {
		if strings.HasPrefix(instanceType, p) && strings.Contains(instanceType, "metal") {
			return true
		}
	}
	return false
}

func vmCapabilityFromReport(report *VMSupportReport) Capability {
	if report == nil {
		return Capability{Name: "vm", Status: "unavailable", Reason: "could not assess VM support"}
	}
	switch report.Status {
	case "supported":
		return Capability{Name: "vm", Status: "available", Reason: report.Reason}
	case "nested_possible":
		return Capability{Name: "vm", Status: "unavailable", Reason: report.Reason + " — " + report.Remediation}
	default:
		return Capability{Name: "vm", Status: "unavailable", Reason: report.Reason}
	}
}
