package machine

import (
	"fmt"
	"math"
)

func incusLaunchCPUFlags(cpu float64, virtualMachine bool) []string {
	if cpu <= 0 {
		cpu = 1
	}
	if virtualMachine {
		n := int(math.Ceil(cpu))
		if n < 1 {
			n = 1
		}
		return []string{fmt.Sprintf("-c limits.cpu=%d", n)}
	}
	if cpu == float64(int(cpu)) {
		n := int(cpu)
		if n < 1 {
			n = 1
		}
		return []string{fmt.Sprintf("-c limits.cpu=%d", n)}
	}
	allowance := int(math.Round(cpu * 100))
	if allowance < 1 {
		allowance = 1
	}
	return []string{fmt.Sprintf("-c limits.cpu.allowance=%d%%", allowance)}
}

// IncusLaunchCPUFlagsForTest exposes CPU flag formatting for tests.
func IncusLaunchCPUFlagsForTest(cpu float64, virtualMachine bool) []string {
	return incusLaunchCPUFlags(cpu, virtualMachine)
}
