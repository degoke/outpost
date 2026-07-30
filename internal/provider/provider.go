package provider

import (
	"context"

	"github.com/degoke/outpost/internal/config"
)

const (
	ProviderAWS = "aws"

	StateRunning      = "running"
	StateStopped      = "stopped"
	StateTerminated   = "terminated"
	StatePending      = "pending"
	StateStopping     = "stopping"
	StateShuttingDown = "shutting-down"
	StateUnknown      = "unknown"
)

type CreateOpts struct {
	Name         string
	Region       string
	Profile      string
	InstanceType string
	SSHPublicKey string
	SSHCIDR      string
	HostID       string
	NoCleanup    bool
}

type CreateResult struct {
	InstanceID    string
	PublicDNS     string
	PublicIP      string
	SecurityGroup string
	VolumeIDs     []string
	InstanceType  string
	Region        string
	Profile       string
}

type InstanceState struct {
	State     string
	PublicDNS string
	PublicIP  string
	VolumeIDs []string
}

type DestroyOpts struct {
	DeleteVolumes bool
}

type ResizeOpts struct {
	InstanceType string
}

type HostProvisioner interface {
	Login(ctx context.Context, profile, region string) (*LoginResult, error)
	Create(ctx context.Context, opts CreateOpts) (*CreateResult, error)
	Describe(ctx context.Context, meta *config.ProviderMeta) (*InstanceState, error)
	Start(ctx context.Context, meta *config.ProviderMeta) error
	Stop(ctx context.Context, meta *config.ProviderMeta) error
	Restart(ctx context.Context, meta *config.ProviderMeta) error
	Resize(ctx context.Context, meta *config.ProviderMeta, opts ResizeOpts) error
	Destroy(ctx context.Context, meta *config.ProviderMeta, opts DestroyOpts) error
	UpdateSSHAccess(ctx context.Context, meta *config.ProviderMeta, cidr string) error
}

type LoginResult struct {
	Account string
	ARN     string
	Region  string
	Profile string
}
