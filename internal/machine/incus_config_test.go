package machine_test

import (
	"testing"

	"github.com/goke/outpost/internal/machine"
	"github.com/stretchr/testify/require"
)

func TestIncusLaunchCPUFlagsFractionalContainer(t *testing.T) {
	flags := machine.IncusLaunchCPUFlagsForTest(0.5, false)
	require.Equal(t, []string{"-c limits.cpu.allowance=50%"}, flags)
}

func TestIncusLaunchCPUFlagsIntegerContainer(t *testing.T) {
	flags := machine.IncusLaunchCPUFlagsForTest(1, false)
	require.Equal(t, []string{"-c limits.cpu=1"}, flags)
}

func TestIncusLaunchCPUFlagsVM(t *testing.T) {
	flags := machine.IncusLaunchCPUFlagsForTest(1, true)
	require.Equal(t, []string{"-c limits.cpu=1"}, flags)
}
