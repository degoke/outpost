package aws

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

type Client struct {
	cfg     aws.Config
	EC2     EC2API
	STS     *sts.Client
	Region  string
	Profile string
}

func NewClient(ctx context.Context, profile, region string) (*Client, error) {
	opts := []func(*awscfg.LoadOptions) error{}
	if profile != "" {
		opts = append(opts, awscfg.WithSharedConfigProfile(profile))
	}
	if region != "" {
		opts = append(opts, awscfg.WithRegion(region))
	}
	cfg, err := awscfg.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}
	if cfg.Region == "" {
		return nil, fmt.Errorf("AWS region is required — set --region or AWS_REGION")
	}
	return &Client{
		cfg:     cfg,
		EC2:     ec2.NewFromConfig(cfg),
		STS:     sts.NewFromConfig(cfg),
		Region:  cfg.Region,
		Profile: profile,
	}, nil
}
func ResolveProfile(profile string) string {
	if profile != "" {
		return profile
	}
	if p := os.Getenv("AWS_PROFILE"); p != "" {
		return p
	}
	return ""
}

func ResolveRegion(region string) string {
	if region != "" {
		return region
	}
	if r := os.Getenv("AWS_REGION"); r != "" {
		return r
	}
	if r := os.Getenv("AWS_DEFAULT_REGION"); r != "" {
		return r
	}
	return ""
}
