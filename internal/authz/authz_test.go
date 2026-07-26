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
	require.False(t, authz.MemberAllowedCommand("invite create"))
}
