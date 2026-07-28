package aws_test

import (
	"context"
	"testing"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/provider"
	"github.com/degoke/outpost/internal/provider/aws"
	"github.com/degoke/outpost/internal/provider/aws/stub"
	"github.com/stretchr/testify/require"
)

func TestLifecycleStartStopResizeDestroy(t *testing.T) {
	ec2 := stub.New()
	prov := aws.NewProvisionerWithEC2("test", "eu-west-1", ec2)

	create, err := prov.Create(context.Background(), provider.CreateOpts{
		Name:         "personal",
		SSHPublicKey: "ssh-ed25519 AAAA test",
		HostID:       "host-12345678-abcd-efgh",
		SSHCIDR:      "203.0.113.1/32",
	})
	require.NoError(t, err)

	meta := &config.ProviderMeta{
		InstanceID:    create.InstanceID,
		InstanceType:  "t3.medium",
		SecurityGroup: create.SecurityGroup,
		VolumeIDs:     create.VolumeIDs,
	}

	require.NoError(t, prov.Stop(context.Background(), meta))
	require.Equal(t, "stopped", ec2.InstanceState(create.InstanceID))

	require.NoError(t, prov.Start(context.Background(), meta))
	require.Equal(t, "running", ec2.InstanceState(create.InstanceID))

	require.NoError(t, prov.Resize(context.Background(), meta, provider.ResizeOpts{InstanceType: "t3.large"}))
	require.Equal(t, "running", ec2.InstanceState(create.InstanceID))

	require.NoError(t, prov.Destroy(context.Background(), meta, provider.DestroyOpts{DeleteVolumes: true}))
	require.Equal(t, "terminated", ec2.InstanceState(create.InstanceID))
	require.Contains(t, ec2.DeletedVolumes, create.VolumeIDs[0])
	require.Contains(t, ec2.DeletedSGs, create.SecurityGroup)
}

func TestDestroyWithoutVolumesKeepsEBS(t *testing.T) {
	ec2 := stub.New()
	prov := aws.NewProvisionerWithEC2("test", "eu-west-1", ec2)
	create, err := prov.Create(context.Background(), provider.CreateOpts{
		Name:         "personal",
		SSHPublicKey: "ssh-ed25519 AAAA test",
		HostID:       "host-12345678-abcd-efgh",
		SSHCIDR:      "203.0.113.1/32",
	})
	require.NoError(t, err)
	meta := &config.ProviderMeta{
		InstanceID:    create.InstanceID,
		SecurityGroup: create.SecurityGroup,
		VolumeIDs:     create.VolumeIDs,
	}
	require.NoError(t, prov.Destroy(context.Background(), meta, provider.DestroyOpts{}))
	require.Empty(t, ec2.DeletedVolumes)
}
