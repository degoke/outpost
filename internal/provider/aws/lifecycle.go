package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/goke/outpost/internal/config"
	"github.com/goke/outpost/internal/provider"
)

func (p *Provisioner) Start(ctx context.Context, meta *config.ProviderMeta) error {
	if meta == nil || meta.InstanceID == "" {
		return fmt.Errorf("host has no AWS instance ID")
	}
	_, err := p.ec2.StartInstances(ctx, &ec2.StartInstancesInput{
		InstanceIds: []string{meta.InstanceID},
	})
	if err != nil {
		return fmt.Errorf("start instance: %w", err)
	}
	_, err = p.waitForRunning(ctx, meta.InstanceID)
	return err
}

func (p *Provisioner) Stop(ctx context.Context, meta *config.ProviderMeta) error {
	if meta == nil || meta.InstanceID == "" {
		return fmt.Errorf("host has no AWS instance ID")
	}
	_, err := p.ec2.StopInstances(ctx, &ec2.StopInstancesInput{
		InstanceIds: []string{meta.InstanceID},
	})
	if err != nil {
		return fmt.Errorf("stop instance: %w", err)
	}
	return p.waitForState(ctx, meta.InstanceID, provider.StateStopped, 5*time.Minute)
}

func (p *Provisioner) Restart(ctx context.Context, meta *config.ProviderMeta) error {
	if meta == nil || meta.InstanceID == "" {
		return fmt.Errorf("host has no AWS instance ID")
	}
	_, err := p.ec2.RebootInstances(ctx, &ec2.RebootInstancesInput{
		InstanceIds: []string{meta.InstanceID},
	})
	if err != nil {
		return fmt.Errorf("reboot instance: %w", err)
	}
	_, err = p.waitForRunning(ctx, meta.InstanceID)
	return err
}

func (p *Provisioner) Resize(ctx context.Context, meta *config.ProviderMeta, opts provider.ResizeOpts) error {
	if meta == nil || meta.InstanceID == "" {
		return fmt.Errorf("host has no AWS instance ID")
	}
	if opts.InstanceType == "" {
		return fmt.Errorf("instance type is required")
	}
	state, err := p.describeInstance(ctx, meta.InstanceID)
	if err != nil {
		return err
	}
	if state.State == provider.StateRunning {
		if err := p.Stop(ctx, meta); err != nil {
			return err
		}
	}
	_, err = p.ec2.ModifyInstanceAttribute(ctx, &ec2.ModifyInstanceAttributeInput{
		InstanceId: aws.String(meta.InstanceID),
		InstanceType: &ec2types.AttributeValue{
			Value: aws.String(opts.InstanceType),
		},
	})
	if err != nil {
		return fmt.Errorf("resize instance: %w", err)
	}
	meta.InstanceType = opts.InstanceType
	return p.Start(ctx, meta)
}

func (p *Provisioner) Destroy(ctx context.Context, meta *config.ProviderMeta, opts provider.DestroyOpts) error {
	if meta == nil || meta.InstanceID == "" {
		return fmt.Errorf("host has no AWS instance ID")
	}
	_, err := p.ec2.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
		InstanceIds: []string{meta.InstanceID},
	})
	if err != nil {
		return fmt.Errorf("terminate instance: %w", err)
	}
	volumes := meta.VolumeIDs
	if opts.DeleteVolumes {
		if len(volumes) == 0 {
			if state, derr := p.describeInstance(ctx, meta.InstanceID); derr == nil {
				volumes = state.VolumeIDs
			}
		}
		_ = p.waitForState(ctx, meta.InstanceID, provider.StateTerminated, 5*time.Minute)
		for _, vol := range volumes {
			_, _ = p.ec2.DeleteVolume(ctx, &ec2.DeleteVolumeInput{VolumeId: aws.String(vol)})
		}
	}
	if meta.SecurityGroup != "" {
		_ = p.waitForState(ctx, meta.InstanceID, provider.StateTerminated, 5*time.Minute)
		_ = p.deleteSecurityGroup(ctx, meta.SecurityGroup)
	}
	return nil
}

func (p *Provisioner) waitForState(ctx context.Context, instanceID, want string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		state, err := p.describeInstance(ctx, instanceID)
		if err != nil {
			return err
		}
		if state.State == want {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
	return fmt.Errorf("timed out waiting for instance %s to reach state %s", instanceID, want)
}
