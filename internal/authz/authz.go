package authz

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/transport"
	"gopkg.in/yaml.v3"
)

func RequireOwner(h *config.Host, action string) error {
	if h.Role != config.RoleOwner {
		return fmt.Errorf("%s is restricted to the host owner — your role is %s", action, h.Role)
	}
	return nil
}

func RequireRuntimeAccess(ctx context.Context, h *config.Host, exec transport.Executor) error {
	if h.Role != config.RoleOwner {
		if h.DeviceID == "" {
			return fmt.Errorf("runtime access not configured — complete 'outpost invite join' and wait for owner approval")
		}
		data, err := exec.Download(config.ShareManifestPath)
		if err != nil {
			return fmt.Errorf("cannot verify device approval: %w", err)
		}
		var m config.ShareManifest
		if err := yaml.Unmarshal(data, &m); err != nil {
			return fmt.Errorf("cannot read share manifest: %w", err)
		}
		d := m.FindDevice(h.DeviceID)
		if d == nil {
			return fmt.Errorf("device %q not registered on host", h.DeviceID)
		}
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
	return nil
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
