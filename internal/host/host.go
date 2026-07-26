package host

import (
	"context"
	"fmt"
	"time"

	"github.com/goke/outpost/internal/authz"
	"github.com/goke/outpost/internal/bootstrap"
	"github.com/goke/outpost/internal/config"
	"github.com/goke/outpost/internal/output"
	"github.com/goke/outpost/internal/transport"
	"github.com/google/uuid"
)

type Service struct {
	Global *config.Global
	Out    *output.Printer
}

func (s *Service) Add(name, hostname, user string, port int, identityFile string) error {
	if name == "" {
		return fmt.Errorf("host name is required")
	}
	if hostname == "" {
		return fmt.Errorf("hostname is required")
	}
	if user == "" {
		return fmt.Errorf("user is required")
	}
	if port == 0 {
		port = 22
	}
	if s.Global.Hosts == nil {
		s.Global.Hosts = map[string]*config.Host{}
	}
	if _, exists := s.Global.Hosts[name]; exists {
		return fmt.Errorf("host %q already exists", name)
	}
	hostID := uuid.New().String()
	s.Global.Hosts[name] = &config.Host{
		Name:         name,
		Hostname:     hostname,
		User:         user,
		Port:         port,
		IdentityFile: identityFile,
		Role:         config.RoleOwner,
		HostID:       hostID,
	}
	if s.Global.ActiveHost == "" {
		s.Global.ActiveHost = name
	}
	return config.SaveGlobal(s.Global)
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
		s.Out.Info("%s %s  %s@%s:%d  role=%s", marker, name, h.User, h.Hostname, h.Port, h.Role)
	}
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
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

func (s *Service) Verify(ctx context.Context, hostName string, skipBootstrap bool) error {
	h, err := s.Global.ResolveHost(hostName)
	if err != nil {
		return err
	}
	exec, err := transport.NewSSH(transport.SSHConfig{
		Hostname:     h.Hostname,
		User:         h.User,
		Port:         h.Port,
		IdentityFile: config.ExpandPath(h.IdentityFile),
	})
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
			if err := bootstrap.Ensure(ctx, exec); err != nil {
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
		s.Out.Info("Checking remote dependencies...")
		if err := bootstrap.Ensure(ctx, exec); err != nil {
			return err
		}
		s.Out.Success("Remote host is ready (Docker and Docker Compose available)")
	}
	return nil
}

func NewExecutor(g *config.Global, hostName string) (transport.Executor, *config.Host, error) {
	h, err := g.ResolveHost(hostName)
	if err != nil {
		return nil, nil, err
	}
	exec, err := transport.NewSSH(transport.SSHConfig{
		Hostname:     h.Hostname,
		User:         h.User,
		Port:         h.Port,
		IdentityFile: config.ExpandPath(h.IdentityFile),
	})
	if err != nil {
		return nil, nil, err
	}
	return exec, h, nil
}
