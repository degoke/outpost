package provider

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goke/outpost/internal/config"
	"golang.org/x/crypto/ssh"
)

func EnsureProvisionKey(hostID string) (pubLine string, privPath string, err error) {
	dir, err := config.IdentitiesDir()
	if err != nil {
		return "", "", err
	}
	hostDir := filepath.Join(dir, hostID)
	if err := os.MkdirAll(hostDir, 0700); err != nil {
		return "", "", err
	}
	privPath = filepath.Join(hostDir, "provision.key")
	pubPath := filepath.Join(hostDir, "provision.pub")
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
	pubLine = strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub)))
	if err := os.WriteFile(pubPath, []byte(pubLine+"\n"), 0644); err != nil {
		return "", "", err
	}
	return pubLine, privPath, nil
}

func ProvisionKeyPath(hostID string) (string, error) {
	dir, err := config.IdentitiesDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, hostID, "provision.key")
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("provision key not found for host %s", hostID)
	}
	return path, nil
}
