package share

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/transport"
	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v3"
)

type Service struct {
	Global *config.Global
	Exec   transport.Executor
	Host   *config.Host
}

func (s *Service) LoadManifest(ctx context.Context) (*config.ShareManifest, error) {
	data, err := s.Exec.Download(config.ShareManifestPath)
	if err != nil {
		return &config.ShareManifest{
			Version:     config.ShareVersion,
			HostID:      s.Host.HostID,
			Invitations: []config.Invitation{},
			Devices:     []config.Device{},
		}, nil
	}
	var m config.ShareManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m.HostID == "" {
		m.HostID = s.Host.HostID
	}
	return &m, nil
}

func (s *Service) SaveManifest(ctx context.Context, m *config.ShareManifest) error {
	m.Version = config.ShareVersion
	data, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	if err := transport.EnsureRemoteDir(s.Exec, filepath.Dir(config.ShareManifestPath)); err != nil {
		return err
	}
	return s.Exec.UploadBytes(data, config.ShareManifestPath)
}

func (s *Service) CreateInvitation(ctx context.Context, ttl time.Duration) (string, error) {
	if err := requireOwner(s.Host); err != nil {
		return "", err
	}
	m, err := s.LoadManifest(ctx)
	if err != nil {
		return "", err
	}
	if m.HostID == "" {
		m.HostID = s.Host.HostID
		if s.Host.HostID == "" {
			m.HostID = uuid.New().String()
			s.Host.HostID = m.HostID
		}
	}
	code := generateCode()
	inv := config.Invitation{
		Code:      code,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(ttl),
	}
	m.Invitations = append(m.Invitations, inv)
	if err := s.SaveManifest(ctx, m); err != nil {
		return "", err
	}
	if s.Host.HostID == "" || s.Host.HostID != m.HostID {
		s.Host.HostID = m.HostID
		if s.Global.Hosts[s.Host.Name] != nil {
			s.Global.Hosts[s.Host.Name].HostID = m.HostID
		}
		_ = config.SaveGlobal(s.Global)
	}
	return code, nil
}

func (s *Service) JoinInvitation(ctx context.Context, code, label, hostname, user string, port int) error {
	m, err := s.LoadManifest(ctx)
	if err != nil {
		return err
	}
	inv := m.FindInvitation(code)
	if inv == nil {
		return fmt.Errorf("invitation code %q not found or expired", code)
	}
	if time.Now().After(inv.ExpiresAt) {
		return fmt.Errorf("invitation code %q has expired", code)
	}
	pubKey, privPath, err := ensureDeviceKey(m.HostID)
	if err != nil {
		return err
	}
	device := config.Device{
		ID:        uuid.New().String(),
		Label:     label,
		PublicKey: pubKey,
		Status:    config.DevicePending,
		JoinedAt:  time.Now().UTC(),
	}
	m.Devices = append(m.Devices, device)
	if err := s.SaveManifest(ctx, m); err != nil {
		return err
	}

	hostName := "shared-" + safePrefix(m.HostID, 8)
	if s.Global.Hosts == nil {
		s.Global.Hosts = map[string]*config.Host{}
	}
	s.Global.Hosts[hostName] = &config.Host{
		Name:         hostName,
		Hostname:     hostname,
		User:         user,
		Port:         port,
		IdentityFile: privPath,
		Role:         config.RoleMember,
		OwnerHostID:  m.HostID,
		HostID:       m.HostID,
		DeviceID:     device.ID,
	}
	s.Global.ActiveHost = hostName
	return config.SaveGlobal(s.Global)
}

func (s *Service) List(ctx context.Context) (*config.ShareManifest, error) {
	return s.LoadManifest(ctx)
}

func (s *Service) Approve(ctx context.Context, deviceID string) error {
	if err := requireOwner(s.Host); err != nil {
		return err
	}
	m, err := s.LoadManifest(ctx)
	if err != nil {
		return err
	}
	d := m.FindDevice(deviceID)
	if d == nil {
		return fmt.Errorf("device %q not found", deviceID)
	}
	if d.Status == config.DeviceApproved {
		return nil
	}
	d.Status = config.DeviceApproved
	if err := s.SaveManifest(ctx, m); err != nil {
		return err
	}
	return syncAuthorizedKeys(ctx, s.Exec, m)
}

func (s *Service) Revoke(ctx context.Context, deviceID string) error {
	if err := requireOwner(s.Host); err != nil {
		return err
	}
	m, err := s.LoadManifest(ctx)
	if err != nil {
		return err
	}
	d := m.FindDevice(deviceID)
	if d == nil {
		return fmt.Errorf("device %q not found", deviceID)
	}
	d.Status = config.DeviceRevoked
	if err := s.SaveManifest(ctx, m); err != nil {
		return err
	}
	return syncAuthorizedKeys(ctx, s.Exec, m)
}

func syncAuthorizedKeys(ctx context.Context, exec transport.Executor, m *config.ShareManifest) error {
	var lines []string
	for _, d := range m.Devices {
		if d.Status == config.DeviceApproved {
			lines = append(lines, strings.TrimSpace(d.PublicKey))
		}
	}
	content := strings.Join(lines, "\n")
	if len(lines) > 0 {
		content += "\n"
	}
	return exec.UploadBytes([]byte(content), config.ShareAuthorizedKeysPath)
}

func requireOwner(h *config.Host) error {
	if h.Role != config.RoleOwner {
		return fmt.Errorf("only the host owner can manage invitations")
	}
	return nil
}

func generateCode() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	enc := strings.ToUpper(base64.RawURLEncoding.EncodeToString(b))
	if len(enc) > 8 {
		enc = enc[:4] + "-" + enc[4:8]
	}
	return enc
}

func safePrefix(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func ensureDeviceKey(hostID string) (pubKey string, privPath string, err error) {
	dir, err := config.IdentitiesDir()
	if err != nil {
		return "", "", err
	}
	hostDir := filepath.Join(dir, hostID)
	if err := os.MkdirAll(hostDir, 0700); err != nil {
		return "", "", err
	}
	privPath = filepath.Join(hostDir, "device.key")
	pubPath := filepath.Join(hostDir, "device.pub")
	if _, err := os.Stat(privPath); err == nil {
		pubBytes, err := os.ReadFile(pubPath)
		if err != nil {
			return "", "", err
		}
		return strings.TrimSpace(string(pubBytes)), privPath, nil
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	privPEM, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(privPath, pem.EncodeToMemory(privPEM), 0600); err != nil {
		return "", "", err
	}
	sshPub, err := ssh.NewPublicKey(priv.Public())
	if err != nil {
		return "", "", err
	}
	pubLine := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub)))
	if err := os.WriteFile(pubPath, []byte(pubLine+"\n"), 0644); err != nil {
		return "", "", err
	}
	return pubLine, privPath, nil
}

func ApprovedCount(ctx context.Context, exec transport.Executor) (int, error) {
	data, err := exec.Download(config.ShareManifestPath)
	if err != nil {
		return 0, nil
	}
	var m config.ShareManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return 0, nil
	}
	return m.ApprovedDeviceCount(), nil
}
