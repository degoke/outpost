package authz_test

import (
	"testing"

	"github.com/goke/outpost/internal/authz"
	"github.com/goke/outpost/internal/config"
	"github.com/stretchr/testify/require"
)

func TestRequireOwner(t *testing.T) {
	owner := &config.Host{Role: config.RoleOwner}
	member := &config.Host{Role: config.RoleMember}
	require.NoError(t, authz.RequireOwner(owner, "invite create"))
	require.Error(t, authz.RequireOwner(member, "invite create"))
}

func TestConfirmDestructiveSkipsWhenAlone(t *testing.T) {
	require.NoError(t, authz.ConfirmDestructive(0, "compose down", false))
}

func TestDenyProviderAndDestroy(t *testing.T) {
	require.Error(t, authz.DenyProviderAndDestroy("host destroy"))
}

func TestMemberAllowedCommands(t *testing.T) {
	require.True(t, authz.MemberAllowedCommand("docker"))
	require.True(t, authz.MemberAllowedCommand("status"))
	require.False(t, authz.MemberAllowedCommand("init"))
}

func TestRequireMemberAllowed(t *testing.T) {
	owner := &config.Host{Role: config.RoleOwner}
	member := &config.Host{Role: config.RoleMember}
	require.NoError(t, authz.RequireMemberAllowed(owner, "host add"))
	require.NoError(t, authz.RequireMemberAllowed(member, "docker"))
	require.NoError(t, authz.RequireMemberAllowed(member, "host verify"))
	require.NoError(t, authz.RequireMemberAllowed(member, "invite join"))
	require.NoError(t, authz.RequireMemberAllowed(member, "prune volumes"))
	require.Error(t, authz.RequireMemberAllowed(member, "host add"))
	require.Error(t, authz.RequireMemberAllowed(member, "init"))
	require.Error(t, authz.RequireMemberAllowed(member, "invite create"))
}
