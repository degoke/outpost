package machine

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	TypeContainer = "container"
	TypeVM        = "vm"

	defaultContainerCPU   = 1
	defaultContainerMem   = uint64(1 * 1024 * 1024 * 1024)
	defaultContainerDisk  = uint64(10 * 1024 * 1024 * 1024)
	defaultVMCPU          = 2
	defaultVMMem          = uint64(2 * 1024 * 1024 * 1024)
	defaultVMDisk         = uint64(20 * 1024 * 1024 * 1024)
	defaultImageContainer = "ubuntu:24.04"
)

type CreateOptions struct {
	Image           string
	CPU             float64
	MemoryBytes     uint64
	DiskBytes       uint64
	VirtualMachine  bool
}

func (o *CreateOptions) ApplyDefaults() {
	if o.Image == "" {
		o.Image = defaultImageContainer
	}
	if o.VirtualMachine {
		if o.CPU == 0 {
			o.CPU = defaultVMCPU
		}
		if o.MemoryBytes == 0 {
			o.MemoryBytes = defaultVMMem
		}
		if o.DiskBytes == 0 {
			o.DiskBytes = defaultVMDisk
		}
	} else {
		if o.CPU == 0 {
			o.CPU = defaultContainerCPU
		}
		if o.MemoryBytes == 0 {
			o.MemoryBytes = defaultContainerMem
		}
		if o.DiskBytes == 0 {
			o.DiskBytes = defaultContainerDisk
		}
	}
}

func (o CreateOptions) MachineType() string {
	if o.VirtualMachine {
		return TypeVM
	}
	return TypeContainer
}

func EstimateResources(opts CreateOptions) (cpu float64, memBytes, diskBytes uint64) {
	opts.ApplyDefaults()
	return opts.CPU, opts.MemoryBytes, opts.DiskBytes
}

func ParseSize(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("size is required")
	}
	upper := strings.ToUpper(s)
	multiplier := uint64(1)
	switch {
	case strings.HasSuffix(upper, "GIB"):
		multiplier = 1024 * 1024 * 1024
		s = s[:len(s)-3]
	case strings.HasSuffix(upper, "MIB"):
		multiplier = 1024 * 1024
		s = s[:len(s)-3]
	case strings.HasSuffix(upper, "GB"):
		multiplier = 1000 * 1000 * 1000
		s = s[:len(s)-2]
	case strings.HasSuffix(upper, "MB"):
		multiplier = 1000 * 1000
		s = s[:len(s)-2]
	case strings.HasSuffix(upper, "G"):
		multiplier = 1024 * 1024 * 1024
		s = s[:len(s)-1]
	case strings.HasSuffix(upper, "M"):
		multiplier = 1024 * 1024
		s = s[:len(s)-1]
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: %w", s, err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("size must be positive")
	}
	return uint64(n * float64(multiplier)), nil
}

func FormatSize(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%dB", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.0f%ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func TypeLabel(machineType string) string {
	switch machineType {
	case TypeVM:
		return "virtual machine (hardware isolated)"
	case TypeContainer:
		return "system container (shared kernel)"
	default:
		return machineType
	}
}
