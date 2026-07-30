package host

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/degoke/outpost/internal/authz"
	"github.com/degoke/outpost/internal/bootstrap"
	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/output"
	"github.com/degoke/outpost/internal/provider"
	"github.com/degoke/outpost/internal/transport"
	"github.com/google/uuid"
)

type Service struct {
	Global *config.Global
	Out    *output.Printer
}

type AddOpts struct {
	Name             string
	Hostname         string
	User             string
	Port             int
	IdentityFile     string
	Password         string
	AuthMode         transport.AuthMode
	SkipBootstrap    bool
	AutoTrustHostKey bool
}

func (s *Service) Add(ctx context.Context, opts AddOpts) error {
	if opts.Name == "" {
		return fmt.Errorf("host name is required")
	}
	if opts.Hostname == "" {
		return fmt.Errorf("hostname is required")
	}
	if opts.User == "" {
		return fmt.Errorf("user is required")
	}
	if opts.Port == 0 {
		opts.Port = 22
	}
	if s.Global.Hosts == nil {
		s.Global.Hosts = map[string]*config.Host{}
	}
	if _, exists := s.Global.Hosts[opts.Name]; exists {
		return fmt.Errorf("host %q already exists", opts.Name)
	}

	authMode := opts.AuthMode
	if authMode == "" {
		authMode = transport.AuthAuto
	}

	sshCfg := transport.SSHConfig{
		Hostname:         opts.Hostname,
		User:             opts.User,
		Port:             opts.Port,
		IdentityFile:     config.ExpandPath(opts.IdentityFile),
		Password:         opts.Password,
		PromptAuth:       true,
		AuthMode:         authMode,
		AutoTrustHostKey: opts.AutoTrustHostKey,
	}
	exec, err := transport.NewSSH(sshCfg)
	if err != nil {
		return err
	}
	defer exec.Close()

	storedIdentity, err := transport.ResolvedIdentityFile(sshCfg)
	if err != nil {
		return err
	}

	if s.Out != nil && !s.Out.JSON {
		s.Out.Info("Verifying SSH connection to %s@%s:%d...", opts.User, opts.Hostname, opts.Port)
	}
	latency, err := transport.Ping(ctx, exec)
	if err != nil {
		return err
	}
	if !opts.SkipBootstrap {
		if err := bootstrap.EnsureWithOut(ctx, exec, s.Out); err != nil {
			return err
		}
	}

	hostID := uuid.New().String()
	s.Global.Hosts[opts.Name] = &config.Host{
		Name:         opts.Name,
		Hostname:     opts.Hostname,
		User:         opts.User,
		Port:         opts.Port,
		IdentityFile: storedIdentity,
		Role:         config.RoleOwner,
		HostID:       hostID,
	}
	if s.Global.ActiveHost == "" {
		s.Global.ActiveHost = opts.Name
	}
	if err := config.SaveGlobal(s.Global); err != nil {
		return err
	}

	if s.Out.JSON {
		return s.Out.PrintJSON(map[string]any{
			"name":          opts.Name,
			"hostname":      opts.Hostname,
			"user":          opts.User,
			"port":          opts.Port,
			"identity_file": storedIdentity,
			"latency":       latency.String(),
			"status":        "verified",
		})
	}
	if !opts.SkipBootstrap {
		s.Out.Success("Host %q verified and registered (%s, %s)", opts.Name, exec.HostInfo(), latency.Round(time.Millisecond))
		s.Out.Info("Remote host is ready (Docker and Docker Compose available)")
		return nil
	}
	s.Out.Success("Host %q verified and registered (%s, %s)", opts.Name, exec.HostInfo(), latency.Round(time.Millisecond))
	return nil
}

func (s *Service) List() error {
	if s.Out.JSON {
		return s.Out.PrintJSON(map[string]any{
			"active_host": s.Global.ActiveHost,
			"hosts":       s.Global.Hosts,
		})
	}
	if len(s.Global.Hosts) == 0 {
		s.Out.Info("No hosts registered. Run: outpost host add NAME --hostname HOST --user USER")
		return nil
	}
	s.Out.Info("Active host: %s", orDash(s.Global.ActiveHost))
	for name, h := range s.Global.Hosts {
		marker := " "
		if name == s.Global.ActiveHost {
			marker = "*"
		}
		s.Out.Info("%s %s  %s@%s:%d  role=%s%s", marker, name, orDash(h.User), orDash(h.Hostname), h.Port, h.Role, cloudHostSuffix(h))
	}
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func cloudHostSuffix(h *config.Host) string {
	if h == nil || h.Provider == nil {
		return ""
	}
	state := strings.TrimSpace(h.Provider.State)
	if state == "" {
		state = provider.StateUnknown
	}
	return fmt.Sprintf("  cloud=%s state=%s", h.Provider.Name, state)
}

func (s *Service) Use(name string) error {
	if _, ok := s.Global.Hosts[name]; !ok {
		return fmt.Errorf("host %q not found", name)
	}
	s.Global.ActiveHost = name
	return config.SaveGlobal(s.Global)
}

func (s *Service) Remove(name string) error {
	h, ok := s.Global.Hosts[name]
	if !ok {
		return fmt.Errorf("host %q not found", name)
	}
	if err := authz.RequireOwner(h, "host remove"); err != nil {
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

func hostSSHConfig(h *config.Host, autoTrustHostKey bool) transport.SSHConfig {
	mode := transport.AuthPassword
	if h.IdentityFile != "" {
		mode = transport.AuthKey
	}
	return transport.SSHConfig{
		Hostname:         h.Hostname,
		User:             h.User,
		Port:             h.Port,
		IdentityFile:     config.ExpandPath(h.IdentityFile),
		PromptAuth:       true,
		AuthMode:         mode,
		AutoTrustHostKey: autoTrustHostKey,
	}
}

func (s *Service) Verify(ctx context.Context, hostName string, skipBootstrap bool, autoTrustHostKey bool) error {
	h, err := s.Global.ResolveHost(hostName)
	if err != nil {
		return err
	}
	exec, err := transport.NewSSH(hostSSHConfig(h, autoTrustHostKey))
	if err != nil {
		return err
	}
	defer exec.Close()

	latency, err := transport.Ping(ctx, exec)
	if err != nil {
		return err
	}
	if s.Out.JSON {
		result := map[string]any{
			"host":    h.Name,
			"status":  "connected",
			"latency": latency.String(),
		}
		if !skipBootstrap {
			if err := bootstrap.EnsureWithOut(ctx, exec, s.Out); err != nil {
				result["bootstrap"] = "failed"
				result["error"] = err.Error()
				_ = s.Out.PrintJSON(result)
				return err
			}
			result["bootstrap"] = "ok"
		}
		return s.Out.PrintJSON(result)
	}
	s.Out.Success("Connected to %s (%s) in %s", h.Name, exec.HostInfo(), latency.Round(time.Millisecond))
	if !skipBootstrap {
		if err := bootstrap.EnsureWithOut(ctx, exec, s.Out); err != nil {
			return err
		}
		s.Out.Success("Remote host is ready (Docker and Docker Compose available)")
	}
	return nil
}

func NewExecutor(g *config.Global, hostName string, autoTrustHostKey bool) (transport.Executor, *config.Host, error) {
	h, err := g.ResolveHost(hostName)
	if err != nil {
		return nil, nil, err
	}
	if err := validateHostConnection(h); err != nil {
		return nil, nil, err
	}
	exec, err := transport.NewSSH(hostSSHConfig(h, autoTrustHostKey))
	if err != nil {
		return nil, nil, err
	}
	return exec, h, nil
}

func validateHostConnection(h *config.Host) error {
	if h == nil {
		return fmt.Errorf("host is required")
	}
	if strings.TrimSpace(h.Hostname) == "" {
		return fmt.Errorf("host %q has no hostname — run 'outpost host list' and use the entry with a full address, or re-register with host add / invite join", h.Name)
	}
	if strings.TrimSpace(h.User) == "" {
		return fmt.Errorf("host %q has no SSH user — check ~/.outpost/config.yaml or re-register the host", h.Name)
	}
	return nil
}
