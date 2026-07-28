package inspect_test

import (
	"testing"

	"github.com/degoke/outpost/internal/inspect"
	"github.com/stretchr/testify/require"
)

func TestParseFreeBytes(t *testing.T) {
	out := `              total        used        free      shared  buff/cache   available
Mem:    8589934592  4294967296  2147483648           0  2147483648  4294967296`
	total, used, avail, err := inspect.ParseFreeBytes(out)
	require.NoError(t, err)
	require.Equal(t, uint64(8589934592), total)
	require.Equal(t, uint64(4294967296), used)
	require.Equal(t, uint64(4294967296), avail)
}

func TestParseDFBytes(t *testing.T) {
	out := `Filesystem      1B-blocks         Used    Available Use% Mounted on
/dev/sda1      100000000000  50000000000  45000000000  53% /`
	total, used, avail, err := inspect.ParseDFBytes(out)
	require.NoError(t, err)
	require.Equal(t, uint64(100000000000), total)
	require.Equal(t, uint64(50000000000), used)
	require.Equal(t, uint64(45000000000), avail)
}

func TestCPUUsagePercent(t *testing.T) {
	pct := inspect.CPUUsagePercent(100, 1000, 600, 2000)
	require.InDelta(t, 50.0, pct, 0.1)
}

func TestParseDockerPSLines(t *testing.T) {
	out := `{"ID":"abc123","Names":"web-1","Image":"nginx","State":"running","Status":"Up","Labels":"com.docker.compose.project=demo,com.docker.compose.service=web"}`
	containers, err := inspect.ParseDockerPSLines(out)
	require.NoError(t, err)
	require.Len(t, containers, 1)
	require.Equal(t, "demo", containers[0].Project)
}

func TestParseDockerSystemDF(t *testing.T) {
	out := `TYPE            TOTAL     ACTIVE    SIZE      RECLAIMABLE
Images          10        5         1.2GB     500MB (41%)
Containers      5         2         100MB     50MB (50%)`
	rows, err := inspect.ParseDockerSystemDF(out)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "Images", rows[0].Type)
}

func TestParseByteQuantity(t *testing.T) {
	require.Equal(t, int64(512*1024*1024), inspect.ParseSizeToBytes("512MiB"))
}

func TestParseDockerImagesLines(t *testing.T) {
	out := `{"Repository":"nginx","Tag":"latest","ID":"abc","Size":"100MB"}
{"Repository":"<none>","Tag":"<none>","ID":"def","Size":"50MB"}`
	images, err := inspect.ParseDockerImagesLines(out)
	require.NoError(t, err)
	require.Len(t, images, 2)
	require.False(t, images[0].Dangling)
	require.Equal(t, "nginx:latest", images[0].RepoTags)
	require.True(t, images[1].Dangling)
}

func TestSumReclaimableBytes(t *testing.T) {
	rows := []inspect.DockerDiskRow{
		{Reclaimable: "500MB (41%)"},
		{Reclaimable: "1.2GB (10%)"},
	}
	require.Equal(t, int64(500*1000*1000+int64(1.2*1000*1000*1000)), inspect.SumReclaimableBytes(rows))
}
