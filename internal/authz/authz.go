package authz

import (
	"bufio"
	"context"
	"crypto"
	"fmt"
	"os"
	"strings"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/transport"
	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v3"
)

func RequireOwner(h *config.Host, action string) error {
	if h == nil {
		return fmt.Errorf("%s is restricted to the host owner — no host is selected", action)
	}
	if h.Role != config.RoleOwner {
		return fmt.Errorf("%s is restricted to the host owner — your role is %s", action, h.Role)
	}
	return nil
}

func RequireRuntimeAccess(ctx context.Context, h *config.Host, exec transport.Executor) error {
	data, err := exec.Download(config.ShareManifestPath)
	if err != nil {
		if h.Role == config.RoleOwner {
			return nil
		}
		return fmt.Errorf("cannot verify device approval: %w", err)
	}
	var m config.ShareManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("cannot read share manifest: %w", err)
	}

	// Do not trust the editable local role field when a device key identifies
	// this connection as a shared member. This also prevents a member from
	// changing role: member to owner in ~/.outpost/config.yaml and reaching
	// owner-only callbacks through the normal SSH executor.
	d := m.FindDevice(h.DeviceID)
	if d == nil {
		if pub, keyErr := publicKeyFromPrivateFile(config.ExpandPath(h.IdentityFile)); keyErr == nil {
			for i := range m.Devices {
				if strings.TrimSpace(m.Devices[i].PublicKey) == pub {
					d = &m.Devices[i]
					break
				}
			}
		}
	}
	if d == nil {
		if h.Role == config.RoleOwner {
			return nil
		}
		return fmt.Errorf("device %q not registered on host", h.DeviceID)
	}
	h.Role = config.RoleMember
	h.DeviceID = d.ID
	switch d.Status {
	case config.DeviceApproved:
		return nil
	case config.DevicePending:
		return fmt.Errorf("device %q is pending owner approval", d.Label)
	case config.DeviceRevoked:
		return fmt.Errorf("device access has been revoked")
	default:
		return fmt.Errorf("device %q is not authorized", d.Label)
	}
}

func publicKeyFromPrivateFile(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("identity file is not configured")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	key, err := ssh.ParseRawPrivateKey(data)
	if err != nil {
		return "", err
	}
	signer, ok := key.(crypto.Signer)
	if !ok {
		return "", fmt.Errorf("unsupported private key type")
	}
	sshPub, err := ssh.NewPublicKey(signer.Public())
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub))), nil
}

func ConfirmDestructive(approvedOthers int, action string, forceYes bool) error {
	if approvedOthers == 0 {
		return nil
	}
	msg := fmt.Sprintf("warning: %d other approved device(s) may be using this host — %s may affect them", approvedOthers, action)
	if forceYes {
		fmt.Fprintf(os.Stderr, "%s (continuing due to --yes)\n", msg)
		return nil
	}
	if !isTerminal() {
		return fmt.Errorf("%s — re-run with --yes to confirm", msg)
	}
	fmt.Fprintf(os.Stderr, "%s\n", msg)
	fmt.Fprintf(os.Stderr, "Continue? [y/N]: ")
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	if line != "y" && line != "yes" {
		return fmt.Errorf("aborted")
	}
	return nil
}

func ConfirmPrompt(message string) error {
	if !isTerminal() {
		return fmt.Errorf("%s — re-run with --yes to confirm", message)
	}
	fmt.Fprintf(os.Stderr, "%s [y/N]: ", message)
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	if line != "y" && line != "yes" {
		return fmt.Errorf("aborted")
	}
	return nil
}

// ConfirmWithYes skips the prompt when forceYes is set.
func ConfirmWithYes(message string, forceYes bool) error {
	if forceYes {
		fmt.Fprintf(os.Stderr, "%s (continuing due to --yes)\n", message)
		return nil
	}
	return ConfirmPrompt(message)
}

func ConfirmYesNo(message string, defaultYes bool) bool {
	if !isTerminal() {
		return defaultYes
	}
	suffix := "[y/N]"
	if defaultYes {
		suffix = "[Y/n]"
	}
	fmt.Fprintf(os.Stderr, "%s %s: ", message, suffix)
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "" {
		return defaultYes
	}
	return line == "y" || line == "yes"
}

func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
