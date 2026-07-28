package integration_test

import (
	"context"
	"testing"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/provider"
	"github.com/degoke/outpost/internal/provider/aws"
	"github.com/degoke/outpost/internal/provider/aws/stub"
	"github.com/stretchr/testify/require"
)

func TestAWSStubCreateAndDestroy(t *testing.T) {
	ec2 := stub.New()
	prov := aws.NewProvisionerWithEC2("test", "eu-west-1", ec2)

	result, err := prov.Create(context.Background(), provider.CreateOpts{
		Name:         "personal",
		SSHPublicKey: "ssh-ed25519 AAAA integration",
		HostID:       "11111111-2222-3333-4444-555555555555",
		SSHCIDR:      "203.0.113.5/32",
	})
	require.NoError(t, err)

	meta := &config.ProviderMeta{
		InstanceID:    result.InstanceID,
		SecurityGroup: result.SecurityGroup,
		VolumeIDs:     result.VolumeIDs,
	}
	require.NoError(t, prov.Destroy(context.Background(), meta, provider.DestroyOpts{DeleteVolumes: true}))
	require.Equal(t, "terminated", ec2.InstanceState(result.InstanceID))
}
