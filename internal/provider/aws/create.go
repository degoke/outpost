package aws

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/provider"
)

const (
	defaultInstanceType = "t3.medium"
	minimumRootVolumeGB = 20
	sshWaitTimeout      = 5 * time.Minute

	ubuntuOwnerID = "099720109477"
)

var ubuntuAMINamePatterns = []string{
	"ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-*",
	"ubuntu/images/hvm-ssd/ubuntu-noble-24.04-amd64-server-*",
}

// UbuntuAMINamePatternsForTest exposes AMI name filters for tests.
func UbuntuAMINamePatternsForTest() []string {
	return append([]string(nil), ubuntuAMINamePatterns...)
}

type rollbackState struct {
	instanceID    string
	securityGroup string
}

func (p *Provisioner) Create(ctx context.Context, opts provider.CreateOpts) (*provider.CreateResult, error) {
	if opts.InstanceType == "" {
		opts.InstanceType = defaultInstanceType
	}
	if opts.SSHPublicKey == "" {
		return nil, fmt.Errorf("SSH public key is required")
	}
	if opts.HostID == "" {
		return nil, fmt.Errorf("host ID is required")
	}

	rb := &rollbackState{}

	ami, err := p.findUbuntuAMI(ctx)
	if err != nil {
		return nil, err
	}

	sgID, err := p.ensureSecurityGroup(ctx, opts)
	if err != nil {
		return nil, err
	}
	rb.securityGroup = sgID

	userData := cloudInitUserData(opts.SSHPublicKey)
	runInput := &ec2.RunInstancesInput{
		ImageId:      aws.String(ami),
		InstanceType: ec2types.InstanceType(opts.InstanceType),
		MinCount:     aws.Int32(1),
		MaxCount:     aws.Int32(1),
		BlockDeviceMappings: []ec2types.BlockDeviceMapping{{
			DeviceName: aws.String("/dev/sda1"),
			Ebs: &ec2types.EbsBlockDevice{
				DeleteOnTermination: aws.Bool(true),
				VolumeSize:          aws.Int32(minimumRootVolumeGB),
				VolumeType:          ec2types.VolumeTypeGp3,
			},
		}},
		UserData:         aws.String(userData),
		SecurityGroupIds: []string{sgID},
		TagSpecifications: []ec2types.TagSpecification{
			{
				ResourceType: ec2types.ResourceTypeInstance,
				Tags: []ec2types.Tag{
					{Key: aws.String("Name"), Value: aws.String("outpost-" + opts.Name)},
					{Key: aws.String("outpost:managed"), Value: aws.String("true")},
					{Key: aws.String("outpost:host-id"), Value: aws.String(opts.HostID)},
				},
			},
		},
	}
	runOut, err := p.ec2.RunInstances(ctx, runInput)
	if err != nil {
		_ = p.deleteSecurityGroup(ctx, sgID)
		return nil, fmt.Errorf("launch EC2 instance: %w", err)
	}
	if len(runOut.Instances) == 0 || runOut.Instances[0].InstanceId == nil {
		_ = p.deleteSecurityGroup(ctx, sgID)
		return nil, fmt.Errorf("EC2 RunInstances returned no instance")
	}
	instanceID := *runOut.Instances[0].InstanceId
	rb.instanceID = instanceID

	state, err := p.waitForRunning(ctx, instanceID)
	if err != nil {
		if !opts.NoCleanup {
			_ = p.terminateInstance(ctx, instanceID)
			_ = p.deleteSecurityGroup(ctx, sgID)
		}
		return nil, err
	}

	volumeIDs := state.VolumeIDs
	return &provider.CreateResult{
		InstanceID:    instanceID,
		PublicDNS:     state.PublicDNS,
		PublicIP:      state.PublicIP,
		SecurityGroup: sgID,
		VolumeIDs:     volumeIDs,
		InstanceType:  opts.InstanceType,
		Region:        p.client.Region,
		Profile:       p.client.Profile,
	}, nil
}

func (p *Provisioner) findUbuntuAMI(ctx context.Context) (string, error) {
	out, err := p.ec2.DescribeImages(ctx, &ec2.DescribeImagesInput{
		Owners: []string{ubuntuOwnerID},
		Filters: []ec2types.Filter{
			{Name: aws.String("name"), Values: ubuntuAMINamePatterns},
			{Name: aws.String("state"), Values: []string{"available"}},
			{Name: aws.String("architecture"), Values: []string{"x86_64"}},
		},
	})
	if err != nil {
		return "", fmt.Errorf("describe Ubuntu AMI: %w", err)
	}
	if len(out.Images) == 0 {
		return "", fmt.Errorf("no Ubuntu 24.04 AMI found in region %s", p.client.Region)
	}
	latest := out.Images[0]
	for _, img := range out.Images[1:] {
		if img.CreationDate != nil && latest.CreationDate != nil && *img.CreationDate > *latest.CreationDate {
			latest = img
		}
	}
	if latest.ImageId == nil {
		return "", fmt.Errorf("AMI lookup failed")
	}
	return *latest.ImageId, nil
}

func (p *Provisioner) ensureSecurityGroup(ctx context.Context, opts provider.CreateOpts) (string, error) {
	sgName := "outpost-" + strings.ReplaceAll(opts.HostID[:8], "-", "")
	vpcOut, err := p.ec2.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{
		Filters: []ec2types.Filter{{Name: aws.String("is-default"), Values: []string{"true"}}},
	})
	if err != nil {
		return "", fmt.Errorf("describe default VPC: %w", err)
	}
	if len(vpcOut.Vpcs) == 0 || vpcOut.Vpcs[0].VpcId == nil {
		return "", fmt.Errorf("no default VPC found in region %s", p.client.Region)
	}
	vpcID := *vpcOut.Vpcs[0].VpcId

	createOut, err := p.ec2.CreateSecurityGroup(ctx, &ec2.CreateSecurityGroupInput{
		GroupName:   aws.String(sgName),
		Description: aws.String("Outpost managed SSH access"),
		VpcId:       aws.String(vpcID),
		TagSpecifications: []ec2types.TagSpecification{
			{
				ResourceType: ec2types.ResourceTypeSecurityGroup,
				Tags: []ec2types.Tag{
					{Key: aws.String("outpost:managed"), Value: aws.String("true")},
					{Key: aws.String("outpost:host-id"), Value: aws.String(opts.HostID)},
				},
			},
		},
	})
	if err != nil {
		if !strings.Contains(err.Error(), "InvalidGroup.Duplicate") {
			return "", fmt.Errorf("create security group: %w", err)
		}
		desc, derr := p.ec2.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
			Filters: []ec2types.Filter{
				{Name: aws.String("group-name"), Values: []string{sgName}},
				{Name: aws.String("vpc-id"), Values: []string{vpcID}},
			},
		})
		if derr != nil || len(desc.SecurityGroups) == 0 {
			return "", fmt.Errorf("security group exists but could not be described: %w", err)
		}
		return *desc.SecurityGroups[0].GroupId, nil
	}

	cidr := opts.SSHCIDR
	if cidr == "" {
		cidr = "0.0.0.0/0"
	}
	_, err = p.ec2.AuthorizeSecurityGroupIngress(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId: createOut.GroupId,
		IpPermissions: []ec2types.IpPermission{
			{
				IpProtocol: aws.String("tcp"),
				FromPort:   aws.Int32(22),
				ToPort:     aws.Int32(22),
				IpRanges:   []ec2types.IpRange{{CidrIp: aws.String(cidr), Description: aws.String("Outpost SSH")}},
			},
		},
	})
	if err != nil && !strings.Contains(err.Error(), "InvalidPermission.Duplicate") {
		_ = p.deleteSecurityGroup(ctx, *createOut.GroupId)
		return "", fmt.Errorf("authorize SSH ingress: %w", err)
	}
	_, err = p.ec2.AuthorizeSecurityGroupEgress(ctx, &ec2.AuthorizeSecurityGroupEgressInput{
		GroupId: createOut.GroupId,
		IpPermissions: []ec2types.IpPermission{
			{
				IpProtocol: aws.String("-1"),
				IpRanges:   []ec2types.IpRange{{CidrIp: aws.String("0.0.0.0/0"), Description: aws.String("Outpost egress")}},
			},
		},
	})
	if err != nil && !strings.Contains(err.Error(), "InvalidPermission.Duplicate") {
		_ = p.deleteSecurityGroup(ctx, *createOut.GroupId)
		return "", fmt.Errorf("authorize egress: %w", err)
	}
	return *createOut.GroupId, nil
}

func (p *Provisioner) waitForRunning(ctx context.Context, instanceID string) (*provider.InstanceState, error) {
	deadline := time.Now().Add(sshWaitTimeout)
	for time.Now().Before(deadline) {
		state, err := p.describeInstance(ctx, instanceID)
		if err != nil {
			return nil, err
		}
		switch state.State {
		case provider.StateRunning:
			if state.PublicDNS != "" || state.PublicIP != "" {
				return state, nil
			}
		case provider.StateTerminated, provider.StateShuttingDown:
			return nil, fmt.Errorf("instance %s entered state %s during launch", instanceID, state.State)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
	return nil, fmt.Errorf("timed out waiting for instance %s to become running with a public address (waited %s)", instanceID, sshWaitTimeout)
}

func (p *Provisioner) Describe(ctx context.Context, meta *config.ProviderMeta) (*provider.InstanceState, error) {
	if meta == nil || meta.InstanceID == "" {
		return nil, fmt.Errorf("host has no AWS instance ID")
	}
	return p.describeInstance(ctx, meta.InstanceID)
}

func (p *Provisioner) describeInstance(ctx context.Context, instanceID string) (*provider.InstanceState, error) {
	out, err := p.ec2.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceID},
	})
	if err != nil {
		return nil, fmt.Errorf("describe instance: %w", err)
	}
	if len(out.Reservations) == 0 || len(out.Reservations[0].Instances) == 0 {
		return nil, fmt.Errorf("instance %s not found", instanceID)
	}
	inst := out.Reservations[0].Instances[0]
	state := provider.StateUnknown
	if inst.State != nil && inst.State.Name != "" {
		state = string(inst.State.Name)
	}
	st := &provider.InstanceState{State: state}
	if inst.PublicDnsName != nil {
		st.PublicDNS = *inst.PublicDnsName
	}
	if inst.PublicIpAddress != nil {
		st.PublicIP = *inst.PublicIpAddress
	}
	for _, bdm := range inst.BlockDeviceMappings {
		if bdm.Ebs != nil && bdm.Ebs.VolumeId != nil {
			st.VolumeIDs = append(st.VolumeIDs, *bdm.Ebs.VolumeId)
		}
	}
	return st, nil
}

func (p *Provisioner) terminateInstance(ctx context.Context, instanceID string) error {
	_, err := p.ec2.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
		InstanceIds: []string{instanceID},
	})
	return err
}

func (p *Provisioner) deleteSecurityGroup(ctx context.Context, sgID string) error {
	_, err := p.ec2.DeleteSecurityGroup(ctx, &ec2.DeleteSecurityGroupInput{
		GroupId: aws.String(sgID),
	})
	return err
}
