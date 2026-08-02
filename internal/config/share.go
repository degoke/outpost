package config

import (
	"strings"
	"time"
)

const (
	ShareManifestPath       = "/var/lib/outpost/share/manifest.yaml"
	ShareAuthorizedKeysPath = "/var/lib/outpost/share/authorized_keys"
)

type DeviceStatus string

const (
	DevicePending  DeviceStatus = "pending"
	DeviceApproved DeviceStatus = "approved"
	DeviceRevoked  DeviceStatus = "revoked"
)

type ShareManifest struct {
	Version     int          `yaml:"version"`
	HostID      string       `yaml:"host_id"`
	Invitations []Invitation `yaml:"invitations"`
	Devices     []Device     `yaml:"devices"`
}

type Invitation struct {
	Code      string    `yaml:"code"`
	CreatedAt time.Time `yaml:"created_at"`
	ExpiresAt time.Time `yaml:"expires_at"`
	UsedAt    time.Time `yaml:"used_at,omitempty"`
}

type Device struct {
	ID        string       `yaml:"id"`
	Label     string       `yaml:"label"`
	PublicKey string       `yaml:"public_key"`
	Status    DeviceStatus `yaml:"status"`
	JoinedAt  time.Time    `yaml:"joined_at"`
}

func (m *ShareManifest) ApprovedDeviceCount() int {
	n := 0
	for _, d := range m.Devices {
		if d.Status == DeviceApproved {
			n++
		}
	}
	return n
}

func (m *ShareManifest) FindDevice(id string) *Device {
	if strings.TrimSpace(id) == "" {
		return nil
	}
	for i := range m.Devices {
		if m.Devices[i].ID == id || strings.HasPrefix(m.Devices[i].ID, id) {
			return &m.Devices[i]
		}
	}
	return nil
}

func (m *ShareManifest) FindInvitation(code string) *Invitation {
	for i := range m.Invitations {
		if m.Invitations[i].Code == code {
			return &m.Invitations[i]
		}
	}
	return nil
}
