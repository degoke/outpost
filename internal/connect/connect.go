package connect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/transport"
	"gopkg.in/yaml.v3"
)

type PortMapping struct {
	Service    string
	HostPort   int
	TargetPort int
	BindHost   string
}

type Session struct {
	Host      string    `json:"host"`
	Project   string    `json:"project"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
	Forwards  []Forward `json:"forwards"`
}

type Forward struct {
	Service    string `json:"service"`
	LocalHost  string `json:"local_host"`
	LocalPort  int    `json:"local_port"`
	RemotePort int    `json:"remote_port"`
	URL        string `json:"url"`
}

func ParseComposePorts(cwd string, proj *config.Project, serviceFilter string) ([]PortMapping, error) {
	var mappings []PortMapping
	for _, rel := range proj.ComposeFiles {
		path := filepath.Join(cwd, rel)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var doc map[string]any
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("parse %s: %w", rel, err)
		}
		services, _ := doc["services"].(map[string]any)
		for svcName, svcVal := range services {
			if serviceFilter != "" && svcName != serviceFilter {
				continue
			}
			svc, _ := svcVal.(map[string]any)
			ports, _ := svc["ports"].([]any)
			for _, p := range ports {
				pm, err := parsePortEntry(svcName, p)
				if err != nil {
					continue
				}
				mappings = append(mappings, pm)
			}
		}
	}
	if len(mappings) == 0 {
		return nil, fmt.Errorf("no published ports found in compose file")
	}
	return mappings, nil
}

// ParseManualPort parses "8080:80" or "8080" into a port mapping.
func ParseManualPort(spec string) (PortMapping, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return PortMapping{}, fmt.Errorf("empty port spec")
	}
	pm, err := parsePortString("manual", spec)
	if err != nil {
		return PortMapping{}, fmt.Errorf("invalid --port %q: %w", spec, err)
	}
	pm.Service = "manual"
	return pm, nil
}

func MergePortMappings(base []PortMapping, manual []PortMapping) []PortMapping {
	if len(manual) == 0 {
		return base
	}
	if len(base) == 0 {
		return manual
	}
	return append(base, manual...)
}

func parsePortEntry(service string, entry any) (PortMapping, error) {
	switch v := entry.(type) {
	case int:
		return PortMapping{Service: service, HostPort: v, TargetPort: v, BindHost: "127.0.0.1"}, nil
	case int64:
		n := int(v)
		return PortMapping{Service: service, HostPort: n, TargetPort: n, BindHost: "127.0.0.1"}, nil
	case string:
		return parsePortString(service, v)
	case map[string]any:
		target := intFromAny(v["target"])
		published := ""
		if p, ok := v["published"].(string); ok {
			published = p
		} else if p := intFromAny(v["published"]); p > 0 {
			published = strconv.Itoa(p)
		}
		if published == "" && target > 0 {
			published = strconv.Itoa(target)
		}
		return parsePortString(service, published+":"+strconv.Itoa(target))
	default:
		return PortMapping{}, fmt.Errorf("unsupported port format")
	}
}

func parsePortString(service, s string) (PortMapping, error) {
	bindHost := "127.0.0.1"
	parts := strings.Split(s, ":")
	var hostPort, targetPort int
	var err error
	switch len(parts) {
	case 1:
		hostPort, err = strconv.Atoi(parts[0])
		targetPort = hostPort
	case 2:
		hostPort, err = strconv.Atoi(parts[0])
		if err != nil {
			return PortMapping{}, err
		}
		targetPort, err = strconv.Atoi(parts[1])
	case 3:
		bindHost = parts[0]
		hostPort, err = strconv.Atoi(parts[1])
		if err != nil {
			return PortMapping{}, err
		}
		targetPort, err = strconv.Atoi(parts[2])
	default:
		return PortMapping{}, fmt.Errorf("invalid port mapping: %s", s)
	}
	if err != nil {
		return PortMapping{}, err
	}
	if hostPort < 1 || hostPort > 65535 || targetPort < 1 || targetPort > 65535 {
		return PortMapping{}, fmt.Errorf("ports must be between 1 and 65535")
	}
	return PortMapping{Service: service, HostPort: hostPort, TargetPort: targetPort, BindHost: bindHost}, nil
}

func intFromAny(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func CheckLocalPort(host string, port int) error {
	if host == "" {
		host = "127.0.0.1"
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		if errors.Is(err, syscall.EADDRINUSE) {
			return fmt.Errorf("local port %d is already in use — try --local-port %d or stop the conflicting process", port, port+1000)
		}
		return fmt.Errorf("cannot bind local port %d: %w", port, err)
	}
	ln.Close()
	return nil
}

func IsProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

// LoadActiveSession returns the session if present and the forwarding process is alive.
// Stale session files are removed automatically.
func LoadActiveSession(host, project string) (*Session, error) {
	sess, err := LoadSession(host, project)
	if err != nil {
		return nil, err
	}
	if !IsProcessAlive(sess.PID) {
		_ = RemoveSession(host, project)
		return nil, fmt.Errorf("no active forwarding session")
	}
	return sess, nil
}

func EnsureNoActiveSession(host, project string) error {
	sess, err := LoadSession(host, project)
	if err != nil {
		return nil
	}
	if IsProcessAlive(sess.PID) {
		return fmt.Errorf("forwarding session already active for %s/%s — run 'outpost connect --down' first", host, project)
	}
	_ = RemoveSession(host, project)
	return nil
}

func SessionPath(host, project string) (string, error) {
	dir, err := config.SessionsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fmt.Sprintf("%s_%s.json", host, project)), nil
}

func SaveSession(sess *Session) error {
	dir, err := config.SessionsDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	path, err := SessionPath(sess.Host, sess.Project)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func LoadSession(host, project string) (*Session, error) {
	path, err := SessionPath(host, project)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, err
	}
	return &sess, nil
}

func RemoveSession(host, project string) error {
	path, err := SessionPath(host, project)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func StartForwards(ctx context.Context, exec transport.Executor, mappings []PortMapping, localPortOverrides map[string]int) ([]Forward, []ioCloser, error) {
	var forwards []Forward
	var closers []ioCloser
	for _, m := range mappings {
		localPort := m.HostPort
		if override, ok := localPortOverrides[m.Service]; ok {
			localPort = override
		}
		bindHost := m.BindHost
		if bindHost == "" {
			bindHost = "127.0.0.1"
		}
		if err := CheckLocalPort(bindHost, localPort); err != nil {
			for _, c := range closers {
				c.Close()
			}
			return nil, nil, err
		}
		closer, err := exec.Forward(ctx, transport.ForwardSpec{
			LocalHost:  bindHost,
			LocalPort:  localPort,
			RemoteHost: "127.0.0.1",
			RemotePort: m.HostPort,
		})
		if err != nil {
			for _, c := range closers {
				c.Close()
			}
			return nil, nil, err
		}
		closers = append(closers, closer)
		fwd := Forward{
			Service:    m.Service,
			LocalHost:  bindHost,
			LocalPort:  localPort,
			RemotePort: m.HostPort,
			URL:        fmt.Sprintf("http://%s:%d", bindHost, localPort),
		}
		forwards = append(forwards, fwd)
	}
	return forwards, closers, nil
}

type ioCloser interface {
	Close() error
}

func StopSession(host, project string) error {
	sess, err := LoadSession(host, project)
	if err != nil {
		return fmt.Errorf("no active forwarding session for %s/%s", host, project)
	}
	if sess.PID > 0 && IsProcessAlive(sess.PID) {
		proc, err := os.FindProcess(sess.PID)
		if err == nil {
			_ = proc.Signal(os.Interrupt)
		}
	}
	return RemoveSession(host, project)
}
