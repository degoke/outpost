package aws_test

import (
	"context"
	"testing"

	awsapi "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/provider"
	"github.com/degoke/outpost/internal/provider/aws"
	"github.com/degoke/outpost/internal/provider/aws/stub"
	"github.com/stretchr/testify/require"
)

func TestUpdateSSHAccessReplacesCIDR(t *testing.T) {
	ec2Stub := stub.New()
	prov := aws.NewProvisionerWithEC2("test", "eu-west-1", ec2Stub)

	create, err := prov.Create(context.Background(), provider.CreateOpts{
		Name:         "personal",
		SSHPublicKey: "ssh-ed25519 AAAA test",
		HostID:       "host-12345678-abcd-efgh",
		SSHCIDR:      "203.0.113.1/32",
	})
	require.NoError(t, err)

	meta := &config.ProviderMeta{SecurityGroup: create.SecurityGroup}
	require.NoError(t, prov.UpdateSSHAccess(context.Background(), meta, "198.51.100.5/32"))

	desc, err := ec2Stub.DescribeSecurityGroups(context.Background(), &ec2.DescribeSecurityGroupsInput{
		GroupIds: []string{create.SecurityGroup},
	})
	require.NoError(t, err)
	require.Len(t, desc.SecurityGroups, 1)

	var cidrs []string
	for _, perm := range desc.SecurityGroups[0].IpPermissions {
		if awsapi.ToString(perm.IpProtocol) != "tcp" {
			continue
		}
		for _, r := range perm.IpRanges {
			cidrs = append(cidrs, awsapi.ToString(r.CidrIp))
		}
	}
	require.Equal(t, []string{"198.51.100.5/32"}, cidrs)
}

func TestUpdateSSHAccessDuplicateCIDR(t *testing.T) {
	ec2Stub := stub.New()
	prov := aws.NewProvisionerWithEC2("test", "eu-west-1", ec2Stub)

	create, err := prov.Create(context.Background(), provider.CreateOpts{
		Name:         "personal",
		SSHPublicKey: "ssh-ed25519 AAAA test",
		HostID:       "host-12345678-abcd-efgh",
		SSHCIDR:      "203.0.113.1/32",
	})
	require.NoError(t, err)

	meta := &config.ProviderMeta{SecurityGroup: create.SecurityGroup}
	require.NoError(t, prov.UpdateSSHAccess(context.Background(), meta, "203.0.113.1/32"))
}

func TestUpdateSSHAccessMissingSecurityGroup(t *testing.T) {
	ec2Stub := stub.New()
	prov := aws.NewProvisionerWithEC2("test", "eu-west-1", ec2Stub)
	err := prov.UpdateSSHAccess(context.Background(), &config.ProviderMeta{}, "203.0.113.1/32")
	require.Error(t, err)
	require.Contains(t, err.Error(), "security group ID")
}

func TestUpdateSSHAccessAuthorizeOnEmptySG(t *testing.T) {
	ec2Stub := stub.New()
	sgOut, err := ec2Stub.CreateSecurityGroup(context.Background(), &ec2.CreateSecurityGroupInput{
		GroupName: awsapi.String("outpost-test"),
		VpcId:     awsapi.String(ec2Stub.DefaultVPC),
	})
	require.NoError(t, err)

	prov := aws.NewProvisionerWithEC2("test", "eu-west-1", ec2Stub)
	meta := &config.ProviderMeta{SecurityGroup: awsapi.ToString(sgOut.GroupId)}
	require.NoError(t, prov.UpdateSSHAccess(context.Background(), meta, "203.0.113.10/32"))

	desc, err := ec2Stub.DescribeSecurityGroups(context.Background(), &ec2.DescribeSecurityGroupsInput{
		GroupIds: []string{meta.SecurityGroup},
	})
	require.NoError(t, err)
	require.Len(t, desc.SecurityGroups[0].IpPermissions, 1)
	require.Equal(t, "203.0.113.10/32", awsapi.ToString(desc.SecurityGroups[0].IpPermissions[0].IpRanges[0].CidrIp))
}
