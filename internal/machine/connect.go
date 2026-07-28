package machine

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/goke/outpost/internal/connect"
	"github.com/goke/outpost/internal/transport"
)

type ConnectForward struct {
	LocalHost  string
	LocalPort  int
	RemotePort int
	URL        string
}

func (s *Service) StartConnect(ctx context.Context, name string, portSpecs []string, bindHost string) ([]ConnectForward, []io.Closer, error) {
	if len(portSpecs) == 0 {
		return nil, nil, fmt.Errorf("at least one --port is required (e.g. --port 8080:80)")
	}
	if bindHost == "" {
		bindHost = "127.0.0.1"
	}
	m, err := s.Status(ctx, name)
	if err != nil {
		return nil, nil, err
	}
	if !strings.EqualFold(m.Status, "running") {
		return nil, nil, fmt.Errorf("machine %q is not running (status: %s)", name, m.Status)
	}
	if m.IPv4 == "" {
		return nil, nil, fmt.Errorf("machine %q has no IP address — try restarting it", name)
	}

	var forwards []ConnectForward
	var closers []io.Closer
	for _, spec := range portSpecs {
		pm, err := connect.ParseManualPort(spec)
		if err != nil {
			for _, c := range closers {
				c.Close()
			}
			return nil, nil, err
		}
		localHost := bindHost
		if pm.BindHost != "" && pm.BindHost != "127.0.0.1" {
			localHost = pm.BindHost
		}
		if err := connect.CheckLocalPort(localHost, pm.HostPort); err != nil {
			for _, c := range closers {
				c.Close()
			}
			return nil, nil, err
		}
		closer, err := s.Exec.Forward(ctx, transport.ForwardSpec{
			LocalHost:  localHost,
			LocalPort:  pm.HostPort,
			RemoteHost: m.IPv4,
			RemotePort: pm.TargetPort,
		})
		if err != nil {
			for _, c := range closers {
				c.Close()
			}
			return nil, nil, err
		}
		closers = append(closers, closer)
		forwards = append(forwards, ConnectForward{
			LocalHost:  localHost,
			LocalPort:  pm.HostPort,
			RemotePort: pm.TargetPort,
			URL:        fmt.Sprintf("http://%s:%d", localHost, pm.HostPort),
		})
	}
	return forwards, closers, nil
}
