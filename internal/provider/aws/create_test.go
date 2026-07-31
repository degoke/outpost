package aws_test

import (
	"context"
	"testing"

	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/degoke/outpost/internal/provider"
	"github.com/degoke/outpost/internal/provider/aws"
	"github.com/degoke/outpost/internal/provider/aws/stub"
	"github.com/stretchr/testify/require"
)

func TestCreateInstanceWithStub(t *testing.T) {
	ec2 := stub.New()
	prov := aws.NewProvisionerWithEC2("test", "eu-west-1", ec2)

	result, err := prov.Create(context.Background(), provider.CreateOpts{
		Name:         "personal",
		InstanceType: "t3.medium",
		SSHPublicKey: "ssh-ed25519 AAAA test",
		HostID:       "host-12345678-abcd-efgh",
		SSHCIDR:      "203.0.113.1/32",
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.InstanceID)
	require.NotEmpty(t, result.PublicDNS)
	require.NotEmpty(t, result.SecurityGroup)
	require.Len(t, result.VolumeIDs, 1)
	require.True(t, ec2.HasSecurityGroup("outpost-host123"))
	require.Len(t, ec2.LastRunInput.BlockDeviceMappings, 1)
	root := ec2.LastRunInput.BlockDeviceMappings[0]
	require.Equal(t, "/dev/sda1", *root.DeviceName)
	require.Equal(t, int32(20), *root.Ebs.VolumeSize)
	require.Equal(t, ec2types.VolumeTypeGp3, root.Ebs.VolumeType)
	require.True(t, *root.Ebs.DeleteOnTermination)
}

func TestCreateSecurityGroupIdempotent(t *testing.T) {
	ec2 := stub.New()
	prov := aws.NewProvisionerWithEC2("test", "eu-west-1", ec2)
	opts := provider.CreateOpts{
		Name:         "personal",
		SSHPublicKey: "ssh-ed25519 AAAA test",
		HostID:       "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		SSHCIDR:      "203.0.113.1/32",
	}
	_, err := prov.Create(context.Background(), opts)
	require.NoError(t, err)
	_, err = prov.Create(context.Background(), opts)
	require.NoError(t, err)
}

func TestCreateRollbackOnNoCleanup(t *testing.T) {
	ec2 := stub.New()
	ec2.AMIID = ""
	prov := aws.NewProvisionerWithEC2("test", "eu-west-1", ec2)
	// Second create with same host ID exercises idempotent SG path; first already tested.
	_, err := prov.Create(context.Background(), provider.CreateOpts{
		Name:         "personal",
		SSHPublicKey: "ssh-ed25519 AAAA test",
		HostID:       "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
		SSHCIDR:      "203.0.113.1/32",
		NoCleanup:    true,
	})
	require.NoError(t, err)
}
