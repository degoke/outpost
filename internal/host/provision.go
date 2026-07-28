package host

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/goke/outpost/internal/authz"
	"github.com/goke/outpost/internal/bootstrap"
	"github.com/goke/outpost/internal/config"
	"github.com/goke/outpost/internal/output"
	"github.com/goke/outpost/internal/provider"
	awsprovider "github.com/goke/outpost/internal/provider/aws"
	"github.com/goke/outpost/internal/transport"
	"github.com/google/uuid"
)

type CreateOpts struct {
	Name         string
	ProviderName string
	Region       string
	Profile      string
	InstanceType string
	SSHCIDR      string
	NoCleanup    bool
}

func (s *Service) Create(ctx context.Context, opts CreateOpts) error {
	if opts.ProviderName != provider.ProviderAWS {
		return fmt.Errorf("unsupported provider %q (supported: aws)", opts.ProviderName)
	}
	if opts.Name == "" {
		return fmt.Errorf("host name is required")
	}
	if _, exists := s.Global.Hosts[opts.Name]; exists {
		return fmt.Errorf("host %q already exists", opts.Name)
	}

	region := awsprovider.ResolveRegion(opts.Region)
	if region == "" {
		region = s.Global.Providers.AWS.DefaultRegion
	}
	if region == "" {
		return fmt.Errorf("region is required — pass --region or run 'outpost provider login aws --region REGION'")
	}
	profile := awsprovider.ResolveProfile(opts.Profile)
	if profile == "" {
		profile = s.Global.Providers.AWS.DefaultProfile
	}

	hostID := uuid.New().String()
	pubKey, privPath, err := provider.EnsureProvisionKey(hostID)
	if err != nil {
		return err
	}

	prov, err := awsprovider.NewProvisioner(ctx, profile, region)
	if err != nil {
		return err
	}
	if _, err := prov.Login(ctx, profile, region); err != nil {
		return err
	}

	sshCIDR := opts.SSHCIDR
	if sshCIDR == "" {
		var detectErr error
		sshCIDR, detectErr = detectPublicIPCIDR()
		if detectErr != nil {
			s.Out.Info("Could not detect public IP for SSH ingress (%v) — pass --ssh-cidr to restrict access", detectErr)
		}
	}

	s.Out.Info("Creating EC2 instance in %s...", region)
	result, err := prov.Create(ctx, provider.CreateOpts{
		Name:         opts.Name,
		Region:       region,
		Profile:      profile,
		InstanceType: opts.InstanceType,
		SSHPublicKey: pubKey,
		SSHCIDR:      sshCIDR,
		HostID:       hostID,
		NoCleanup:    opts.NoCleanup,
	})
	if err != nil {
		return err
	}

	hostname := result.PublicDNS
	if hostname == "" {
		hostname = result.PublicIP
	}
	s.Out.Info("Waiting for SSH on %s...", hostname)
	if err := waitForSSH(ctx, hostname, outpostUser, privPath); err != nil {
		if !opts.NoCleanup {
			s.Out.Info("Cleaning up failed instance...")
			_ = prov.Destroy(ctx, &config.ProviderMeta{
				InstanceID:    result.InstanceID,
				SecurityGroup: result.SecurityGroup,
				VolumeIDs:     result.VolumeIDs,
			}, provider.DestroyOpts{})
		}
		return err
	}

	s.Out.Info("Bootstrapping remote host...")
	exec, err := transport.NewSSH(provisionSSHConfig(hostname, outpostUser, privPath))
	if err != nil {
		return err
	}
	defer exec.Close()
	if err := bootstrap.Ensure(ctx, exec); err != nil {
		if !opts.NoCleanup {
			_ = prov.Destroy(ctx, &config.ProviderMeta{
				InstanceID:    result.InstanceID,
				SecurityGroup: result.SecurityGroup,
				VolumeIDs:     result.VolumeIDs,
			}, provider.DestroyOpts{})
		}
		return err
	}

	if s.Global.Hosts == nil {
		s.Global.Hosts = map[string]*config.Host{}
	}
	s.Global.Hosts[opts.Name] = &config.Host{
		Name:         opts.Name,
		Hostname:     hostname,
		User:         outpostUser,
		Port:         22,
		IdentityFile: privPath,
		Role:         config.RoleOwner,
		HostID:       hostID,
		Provider: &config.ProviderMeta{
			Name:          provider.ProviderAWS,
			Region:        result.Region,
			Profile:       result.Profile,
			InstanceID:    result.InstanceID,
			InstanceType:  result.InstanceType,
			SecurityGroup: result.SecurityGroup,
			VolumeIDs:     result.VolumeIDs,
			State:         provider.StateRunning,
		},
	}
	s.Global.ActiveHost = opts.Name
	if err := config.SaveGlobal(s.Global); err != nil {
		return err
	}
	if s.Out.JSON {
		return s.Out.PrintJSON(s.Global.Hosts[opts.Name])
	}
	s.Out.Success("Host %q created and ready at %s", opts.Name, hostname)
	return nil
}

func (s *Service) Start(ctx context.Context, name string) error {
	h, prov, err := s.awsProvisioner(ctx, name)
	if err != nil {
		return err
	}
	if err := prov.Start(ctx, h.Provider); err != nil {
		return err
	}
	if state, err := prov.Describe(ctx, h.Provider); err == nil {
		h.Hostname = firstNonEmpty(state.PublicDNS, state.PublicIP, h.Hostname)
		h.Provider.State = state.State
	}
	return config.SaveGlobal(s.Global)
}

func (s *Service) Stop(ctx context.Context, name string) error {
	h, prov, err := s.awsProvisioner(ctx, name)
	if err != nil {
		return err
	}
	if err := prov.Stop(ctx, h.Provider); err != nil {
		return err
	}
	h.Provider.State = provider.StateStopped
	return config.SaveGlobal(s.Global)
}

func (s *Service) Restart(ctx context.Context, name string) error {
	h, prov, err := s.awsProvisioner(ctx, name)
	if err != nil {
		return err
	}
	if err := prov.Restart(ctx, h.Provider); err != nil {
		return err
	}
	if state, err := prov.Describe(ctx, h.Provider); err == nil {
		h.Hostname = firstNonEmpty(state.PublicDNS, state.PublicIP, h.Hostname)
		h.Provider.State = state.State
	}
	return config.SaveGlobal(s.Global)
}

func (s *Service) Resize(ctx context.Context, name, instanceType string) error {
	h, prov, err := s.awsProvisioner(ctx, name)
	if err != nil {
		return err
	}
	if err := prov.Resize(ctx, h.Provider, provider.ResizeOpts{InstanceType: instanceType}); err != nil {
		return err
	}
	h.Provider.InstanceType = instanceType
	h.Provider.State = provider.StateRunning
	return config.SaveGlobal(s.Global)
}

func (s *Service) Destroy(ctx context.Context, name string, deleteVolumes, forceYes bool) error {
	h, ok := s.Global.Hosts[name]
	if !ok {
		return fmt.Errorf("host %q not found", name)
	}
	if err := authz.RequireOwner(h, "host destroy"); err != nil {
		return err
	}
	if h.Provider == nil || h.Provider.Name != provider.ProviderAWS {
		return fmt.Errorf("host %q is not a cloud-managed host — use 'outpost host remove' to drop local config only", name)
	}
	if !forceYes {
		if err := authz.ConfirmPrompt("This will terminate the EC2 instance and destroy all runtime data on it"); err != nil {
			return err
		}
		if !deleteVolumes {
			deleteVolumes = authz.ConfirmYesNo("Also delete attached EBS volumes? This cannot be undone", false)
		}
	}
	prov, err := awsprovider.NewProvisioner(ctx, h.Provider.Profile, h.Provider.Region)
	if err != nil {
		return err
	}
	if err := prov.Destroy(ctx, h.Provider, provider.DestroyOpts{DeleteVolumes: deleteVolumes}); err != nil {
		return err
	}
	delete(s.Global.Hosts, name)
	if s.Global.ActiveHost == name {
		s.Global.ActiveHost = ""
		for n := range s.Global.Hosts {
			s.Global.ActiveHost = n
			break
		}
	}
	return config.SaveGlobal(s.Global)
}

func (s *Service) awsProvisioner(ctx context.Context, name string) (*config.Host, *awsprovider.Provisioner, error) {
	h, ok := s.Global.Hosts[name]
	if !ok {
		return nil, nil, fmt.Errorf("host %q not found", name)
	}
	if err := authz.RequireOwner(h, "host lifecycle"); err != nil {
		return nil, nil, err
	}
	if h.Provider == nil || h.Provider.Name != provider.ProviderAWS {
		return nil, nil, fmt.Errorf("host %q is not a cloud-managed AWS host", name)
	}
	prov, err := awsprovider.NewProvisioner(ctx, h.Provider.Profile, h.Provider.Region)
	if err != nil {
		return nil, nil, err
	}
	return h, prov, nil
}

const outpostUser = "outpost"

const provisionSSHTimeout = 10 * time.Minute

func provisionSSHConfig(hostname, user, identityFile string) transport.SSHConfig {
	return transport.SSHConfig{
		Hostname:         hostname,
		User:             user,
		Port:             22,
		IdentityFile:     identityFile,
		AutoTrustHostKey: true,
	}
}

func waitForSSH(ctx context.Context, hostname, user, identityFile string) error {
	deadline := time.Now().Add(provisionSSHTimeout)
	var lastErr error
	cfg := provisionSSHConfig(hostname, user, identityFile)
	for time.Now().Before(deadline) {
		exec, err := transport.NewSSH(cfg)
		if err == nil {
			var stdout bytes.Buffer
			code, runErr := exec.Run(ctx, "echo OUTPOST_SSH_OK", transport.RunOpts{
				Stdout: &stdout,
				Stderr: io.Discard,
			})
			exec.Close()
			if runErr == nil && code == 0 && strings.TrimSpace(stdout.String()) == "OUTPOST_SSH_OK" {
				return nil
			}
			if runErr != nil {
				lastErr = runErr
			} else {
				lastErr = fmt.Errorf("unexpected SSH probe response: %q", stdout.String())
			}
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
	if lastErr != nil {
		return fmt.Errorf("SSH not ready after %s: %w", provisionSSHTimeout, lastErr)
	}
	return fmt.Errorf("SSH not ready after %s connecting to %s@%s", provisionSSHTimeout, user, hostname)
}

func detectPublicIPCIDR() (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://checkip.amazonaws.com")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	ip := strings.TrimSpace(string(data))
	if ip == "" {
		return "", fmt.Errorf("could not detect public IP")
	}
	return ip + "/32", nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func ProviderLogin(ctx context.Context, g *config.Global, out *output.Printer, profile, region string) error {
	profile = awsprovider.ResolveProfile(profile)
	if profile == "" {
		profile = g.Providers.AWS.DefaultProfile
	}
	region = awsprovider.ResolveRegion(region)
	if region == "" {
		region = g.Providers.AWS.DefaultRegion
	}
	if region == "" {
		return fmt.Errorf("region is required")
	}
	prov, err := awsprovider.NewProvisioner(ctx, profile, region)
	if err != nil {
		return err
	}
	result, err := prov.Login(ctx, profile, region)
	if err != nil {
		return err
	}
	g.Providers.AWS.DefaultProfile = result.Profile
	g.Providers.AWS.DefaultRegion = result.Region
	if err := config.SaveGlobal(g); err != nil {
		return err
	}
	if out.JSON {
		return out.PrintJSON(result)
	}
	out.Success("AWS login OK (account %s, region %s)", result.Account, result.Region)
	out.Info("ARN: %s", result.ARN)
	return nil
}
