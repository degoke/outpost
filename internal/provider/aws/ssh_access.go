package aws

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/degoke/outpost/internal/config"
)

func (p *Provisioner) UpdateSSHAccess(ctx context.Context, meta *config.ProviderMeta, cidr string) error {
	if meta == nil || meta.SecurityGroup == "" {
		return fmt.Errorf("security group ID is not set for this host")
	}
	if strings.TrimSpace(cidr) == "" {
		return fmt.Errorf("SSH CIDR is required")
	}

	desc, err := p.ec2.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
		GroupIds: []string{meta.SecurityGroup},
	})
	if err != nil {
		return fmt.Errorf("describe security group: %w", err)
	}
	if len(desc.SecurityGroups) == 0 {
		return fmt.Errorf("security group %s not found", meta.SecurityGroup)
	}
	sg := desc.SecurityGroups[0]

	var revokePerms []ec2types.IpPermission
	for _, perm := range sg.IpPermissions {
		if !isSSHPermission(perm) {
			continue
		}
		if len(perm.IpRanges) == 0 {
			continue
		}
		revokePerms = append(revokePerms, ec2types.IpPermission{
			IpProtocol: perm.IpProtocol,
			FromPort:   perm.FromPort,
			ToPort:     perm.ToPort,
			IpRanges:   perm.IpRanges,
		})
	}
	if len(revokePerms) > 0 {
		_, err = p.ec2.RevokeSecurityGroupIngress(ctx, &ec2.RevokeSecurityGroupIngressInput{
			GroupId:       sg.GroupId,
			IpPermissions: revokePerms,
		})
		if err != nil {
			return fmt.Errorf("revoke SSH ingress: %w", err)
		}
	}

	_, err = p.ec2.AuthorizeSecurityGroupIngress(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId: sg.GroupId,
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
		return fmt.Errorf("authorize SSH ingress: %w", err)
	}
	return nil
}

func isSSHPermission(perm ec2types.IpPermission) bool {
	if aws.ToString(perm.IpProtocol) != "tcp" {
		return false
	}
	from := aws.ToInt32(perm.FromPort)
	to := aws.ToInt32(perm.ToPort)
	return from <= 22 && to >= 22
}
