package aws

import (
	"context"
	"fmt"

	"github.com/degoke/outpost/internal/provider"
)

type Provisioner struct {
	client *Client
	ec2    EC2API
}

var _ provider.HostProvisioner = (*Provisioner)(nil)

func NewProvisioner(ctx context.Context, profile, region string) (*Provisioner, error) {
	client, err := NewClient(ctx, profile, region)
	if err != nil {
		return nil, err
	}
	return &Provisioner{client: client, ec2: client.EC2}, nil
}

// NewProvisionerWithEC2 creates a provisioner backed by a custom EC2 API (for tests).
func NewProvisionerWithEC2(profile, region string, ec2 EC2API) *Provisioner {
	return &Provisioner{
		client: &Client{EC2: ec2, Region: region, Profile: profile},
		ec2:    ec2,
	}
}

func (p *Provisioner) Login(ctx context.Context, profile, region string) (*provider.LoginResult, error) {
	if region != "" && region != p.client.Region {
		c, err := NewClient(ctx, profile, region)
		if err != nil {
			return nil, err
		}
		p.client = c
	}
	out, err := p.client.STS.GetCallerIdentity(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("AWS credentials invalid: %w", err)
	}
	account := ""
	if out.Account != nil {
		account = *out.Account
	}
	arn := ""
	if out.Arn != nil {
		arn = *out.Arn
	}
	return &provider.LoginResult{
		Account: account,
		ARN:     arn,
		Region:  p.client.Region,
		Profile: p.client.Profile,
	}, nil
}
