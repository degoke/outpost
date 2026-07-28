package aws_test

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/goke/outpost/internal/provider/aws"
	"github.com/stretchr/testify/require"
)

func TestCloudInitUserData(t *testing.T) {
	data := aws.CloudInitUserDataForTest("ssh-ed25519 AAAA test")
	require.NotEmpty(t, data)
	decoded, err := base64.StdEncoding.DecodeString(data)
	require.NoError(t, err)
	script := string(decoded)
	require.Contains(t, script, "ssh-ed25519 AAAA test")
	require.Contains(t, script, "outpost")
	require.Contains(t, script, "machines")
	require.Contains(t, script, "OUTPOST_CLOUD_INIT_OK")
	require.NotContains(t, script, "install_docker_debian")
}

func TestCloudInitIsBase64(t *testing.T) {
	data := aws.CloudInitUserDataForTest("key")
	require.True(t, len(data) > 20)
	require.NotContains(t, strings.TrimSpace(data), "\n")
}
