package authz

import (
	"fmt"
	"strings"

	"github.com/goke/outpost/internal/config"
)

// RequireMemberAllowed checks whether a member-role host may run the given command path.
// Owners always pass. cmdPath uses space-separated segments, e.g. "host add" or "compose up".
func RequireMemberAllowed(h *config.Host, cmdPath string) error {
	if h == nil || h.Role != config.RoleMember {
		return nil
	}
	if memberAllowedPath(cmdPath) {
		return nil
	}
	return fmt.Errorf("%q is restricted to the host owner — your role is %s", cmdPath, h.Role)
}

func memberAllowedPath(cmdPath string) bool {
	parts := strings.Fields(cmdPath)
	if len(parts) == 0 {
		return true
	}
	root := parts[0]
	switch root {
	case "docker", "compose", "connect", "status", "top", "capacity", "disk", "prune":
		return true
	case "host":
		if len(parts) < 2 {
			return false
		}
		switch parts[1] {
		case "verify", "list":
			return true
		default:
			return false
		}
	case "invite":
		if len(parts) >= 2 && parts[1] == "join" {
			return true
		}
		return false
	default:
		return false
	}
}

// MemberAllowedCommand reports whether a top-level command name is allowed for members.
func MemberAllowedCommand(cmd string) bool {
	return memberAllowedPath(cmd)
}

// ResolveActiveHostRole returns the role for the active or flagged host, defaulting to owner when unknown.
func ResolveActiveHostRole(g *config.Global, hostFlag string) config.Role {
	if g == nil {
		return config.RoleOwner
	}
	name := hostFlag
	if name == "" {
		name = g.ActiveHost
	}
	if name == "" {
		return config.RoleOwner
	}
	h, ok := g.Hosts[name]
	if !ok || h.Role == "" {
		return config.RoleOwner
	}
	return h.Role
}
