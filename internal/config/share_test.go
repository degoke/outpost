package config_test

import (
	"testing"

	"github.com/degoke/outpost/internal/config"
	"github.com/stretchr/testify/require"
)

func TestFindDeviceDoesNotMatchEmptyID(t *testing.T) {
	m := config.ShareManifest{Devices: []config.Device{{ID: "device-1", Status: config.DeviceApproved}}}
	require.Nil(t, m.FindDevice(""))
}
