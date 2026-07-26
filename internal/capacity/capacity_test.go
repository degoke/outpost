package capacity_test

import (
	"testing"

	"github.com/goke/outpost/internal/capacity"
	"github.com/goke/outpost/internal/inspect"
	"github.com/stretchr/testify/require"
)

func TestAvailableAfterMargin(t *testing.T) {
	avail := capacity.AvailableAfterMargin(800, 1000)
	require.Equal(t, uint64(700), avail)
}

func TestCheckInsufficientMemory(t *testing.T) {
	err := capacity.CheckWithReport(&capacity.Report{
		Host: inspect.HostMetrics{
			MemoryTotal: 8 * 1024 * 1024 * 1024,
			MemoryUsed:  7 * 1024 * 1024 * 1024,
		},
		AvailableMem: 512 * 1024 * 1024,
	}, capacity.Request{MemoryBytes: 4 * 1024 * 1024 * 1024})
	require.Error(t, err)
	require.Contains(t, err.Error(), "insufficient memory")
}
