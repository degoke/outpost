package cluster_test

import (
	"testing"

	"github.com/degoke/outpost/internal/cluster"
	"github.com/stretchr/testify/require"
)

func TestParseDriver(t *testing.T) {
	d, err := cluster.ParseDriver("kind")
	require.NoError(t, err)
	require.Equal(t, cluster.DriverKind, d)

	d, err = cluster.ParseDriver("k3d")
	require.NoError(t, err)
	require.Equal(t, cluster.DriverK3d, d)

	d, err = cluster.ParseDriver("")
	require.NoError(t, err)
	require.Equal(t, cluster.DriverKind, d)

	_, err = cluster.ParseDriver("minikube")
	require.Error(t, err)
}
