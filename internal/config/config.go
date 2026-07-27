package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"gopkg.in/yaml.v3"
)

const (
	ConfigVersion     = 1
	ProjectVersion    = 1
	ShareVersion      = 1
	DefaultRemoteBase = "/var/lib/outpost"
)

type Role string

const (
	RoleOwner  Role = "owner"
	RoleMember Role = "member"
)

type Global struct {
	Version    int              `yaml:"version"`
	ActiveHost string           `yaml:"active_host"`
	Hosts      map[string]*Host `yaml:"hosts"`
	Providers  ProvidersConfig  `yaml:"providers,omitempty"`
}

type ProvidersConfig struct {
	AWS AWSProviderConfig `yaml:"aws,omitempty"`
}

type AWSProviderConfig struct {
	DefaultProfile string `yaml:"default_profile,omitempty"`
	DefaultRegion  string `yaml:"default_region,omitempty"`
}

type ProviderMeta struct {
	Name          string   `yaml:"name"`
	Region        string   `yaml:"region"`
	Profile       string   `yaml:"profile,omitempty"`
	InstanceID    string   `yaml:"instance_id"`
	InstanceType  string   `yaml:"instance_type"`
	SecurityGroup string   `yaml:"security_group_id,omitempty"`
	VolumeIDs     []string `yaml:"volume_ids,omitempty"`
	State         string   `yaml:"state,omitempty"`
}

type Host struct {
	Name         string        `yaml:"-"`
	Hostname     string        `yaml:"hostname"`
	User         string        `yaml:"user"`
	Port         int           `yaml:"port"`
	IdentityFile string        `yaml:"identity_file"`
	Role         Role          `yaml:"role"`
	OwnerHostID  string        `yaml:"owner_host_id,omitempty"`
	HostID       string        `yaml:"host_id,omitempty"`
	DeviceID     string        `yaml:"device_id,omitempty"`
	Provider     *ProviderMeta `yaml:"provider,omitempty"`
}

type Project struct {
	Version      int      `yaml:"version"`
	Name         string   `yaml:"name"`
	Host         string   `yaml:"host,omitempty"`
	RemoteDir    string   `yaml:"remote_dir"`
	ComposeFiles []string `yaml:"compose_files"`
	ExtraFiles   []string `yaml:"extra_files,omitempty"`
}

func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".outpost"), nil
}

func GlobalConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

func ProjectConfigPath(cwd string) string {
	return filepath.Join(cwd, ".outpost", "project.yaml")
}

func SessionsDir() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sessions"), nil
}

func IdentitiesDir() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "identities"), nil
}

func LoadGlobal() (*Global, error) {
	path, err := GlobalConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Global{Version: ConfigVersion, Hosts: map[string]*Host{}}, nil
		}
		return nil, err
	}
	var g Global
	if err := yaml.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if g.Hosts == nil {
		g.Hosts = map[string]*Host{}
	}
	for name, h := range g.Hosts {
		h.Name = name
		if h.Role == "" {
			h.Role = RoleOwner
		}
		if h.Port == 0 {
			h.Port = 22
		}
	}
	return &g, nil
}

func SaveGlobal(g *Global) error {
	path, err := GlobalConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	g.Version = ConfigVersion
	data, err := yaml.Marshal(g)
	if err != nil {
		return err
	}
	return writeLockedAtomic(path, data, 0600)
}

func (g *Global) ResolveHost(name string) (*Host, error) {
	hostName := name
	if hostName == "" {
		hostName = g.ActiveHost
	}
	if hostName == "" {
		return nil, fmt.Errorf("no host selected: run 'outpost host use NAME' or pass --host")
	}
	h, ok := g.Hosts[hostName]
	if !ok {
		return nil, fmt.Errorf("host %q not found", hostName)
	}
	h.Name = hostName
	return h, nil
}

func LoadProject(cwd string) (*Project, error) {
	path := ProjectConfigPath(cwd)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("project not initialized: run 'outpost init' in this directory")
		}
		return nil, err
	}
	var p Project
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse project %s: %w", path, err)
	}
	return &p, nil
}

func SaveProject(cwd string, p *Project) error {
	dir := filepath.Join(cwd, ".outpost")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	p.Version = ProjectVersion
	if p.RemoteDir == "" {
		p.RemoteDir = filepath.Join(DefaultRemoteBase, "projects", p.Name)
	}
	// Normalize to Unix paths for remote
	p.RemoteDir = strings.ReplaceAll(p.RemoteDir, "\\", "/")
	data, err := yaml.Marshal(p)
	if err != nil {
		return err
	}
	return writeLockedAtomic(ProjectConfigPath(cwd), data, 0644)
}

// writeLockedAtomic serializes local metadata updates and replaces the target
// in one rename, so concurrent commands cannot observe a partially-written
// configuration file.
func writeLockedAtomic(path string, data []byte, mode os.FileMode) error {
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".outpost-write-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func ExpandPath(p string) string {
	if p == "" {
		return p
	}
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return filepath.Join(home, p[2:])
	}
	return p
}

func KubeconfigsDir() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "kubeconfigs"), nil
}

func SanitizeClusterName(name string) string {
	return SanitizeProjectName(name)
}

func SanitizeMachineName(name string) string {
	return SanitizeProjectName(name)
}

func SanitizeProjectName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune('-')
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "project"
	}
	return out
}
