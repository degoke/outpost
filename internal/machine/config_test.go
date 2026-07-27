package machine_test

import (
	"testing"

	"github.com/goke/outpost/internal/machine"
	"github.com/stretchr/testify/require"
)

func TestApplyDefaultsContainer(t *testing.T) {
	opts := machine.CreateOptions{Image: "ubuntu:24.04"}
	opts.ApplyDefaults()
	require.Equal(t, 0.5, opts.CPU)
	require.Equal(t, uint64(128*1024*1024), opts.MemoryBytes)
	require.Equal(t, uint64(2*1024*1024*1024), opts.DiskBytes)
	require.Equal(t, machine.TypeContainer, opts.MachineType())
}

func TestApplyDefaultsVM(t *testing.T) {
	opts := machine.CreateOptions{Image: "ubuntu:24.04", VirtualMachine: true}
	opts.ApplyDefaults()
	require.Equal(t, float64(1), opts.CPU)
	require.Equal(t, uint64(256*1024*1024), opts.MemoryBytes)
	require.Equal(t, uint64(3*1024*1024*1024), opts.DiskBytes)
	require.Equal(t, machine.TypeVM, opts.MachineType())
}
