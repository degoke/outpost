package machine_test

import (
	"testing"

	"github.com/degoke/outpost/internal/machine"
	"github.com/stretchr/testify/require"
)

func TestIncusImagePullSpecUbuntuAlias(t *testing.T) {
	remote, alias, ok := machine.IncusImagePullSpecForTest("ubuntu:24.04")
	require.True(t, ok)
	require.Equal(t, "images:ubuntu/24.04", remote)
	require.Equal(t, "ubuntu:24.04", alias)
}

func TestIncusImagePullSpecRemoteRef(t *testing.T) {
	remote, alias, ok := machine.IncusImagePullSpecForTest("images:ubuntu/24.04")
	require.True(t, ok)
	require.Equal(t, "images:ubuntu/24.04", remote)
	require.Equal(t, "", alias)
}

func TestIncusImagePullSpecPath(t *testing.T) {
	remote, alias, ok := machine.IncusImagePullSpecForTest("ubuntu/24.04")
	require.True(t, ok)
	require.Equal(t, "images:ubuntu/24.04", remote)
	require.Equal(t, "ubuntu/24.04", alias)
}

func TestIncusLocalImageRefUbuntuAlias(t *testing.T) {
	require.Equal(t, "local:ubuntu:24.04", machine.IncusLocalImageRefForTest("ubuntu:24.04"))
}

func TestIncusLocalImageRefRemotePassthrough(t *testing.T) {
	require.Equal(t, "images:ubuntu/24.04", machine.IncusLocalImageRefForTest("images:ubuntu/24.04"))
	require.Equal(t, "local:ubuntu:24.04", machine.IncusLocalImageRefForTest("local:ubuntu:24.04"))
}
