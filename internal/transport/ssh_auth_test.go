package transport_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/goke/outpost/internal/transport"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestDefaultIdentityFileFindsKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	require.NoError(t, os.MkdirAll(sshDir, 0700))
	keyPath := filepath.Join(sshDir, "id_ed25519")
	require.NoError(t, os.WriteFile(keyPath, []byte("key"), 0600))

	path, err := transport.DefaultIdentityFile()
	require.NoError(t, err)
	require.Equal(t, keyPath, path)
}

func TestBuildAuthPasswordOnly(t *testing.T) {
	methods, err := transport.BuildAuthForTest(transport.SSHConfig{
		User:       "ubuntu",
		Hostname:   "example.com",
		AuthMode:   transport.AuthPassword,
		Password:   "secret",
		PromptAuth: false,
	})
	require.NoError(t, err)
	require.Len(t, methods, 1)
}

func TestBuildAuthKeyFile(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pemBlock, err := ssh.MarshalPrivateKey(key, "")
	require.NoError(t, err)
	pemBytes := pem.EncodeToMemory(pemBlock)

	dir := t.TempDir()
	keyPath := filepath.Join(dir, "vps_key")
	require.NoError(t, os.WriteFile(keyPath, pemBytes, 0600))

	methods, err := transport.BuildAuthForTest(transport.SSHConfig{
		IdentityFile: keyPath,
		User:         "ubuntu",
		Hostname:     "example.com",
		AuthMode:     transport.AuthKey,
	})
	require.NoError(t, err)
	require.NotEmpty(t, methods)
}

func TestBuildAuthAutoWithoutIdentityUsesPasswordOnly(t *testing.T) {
	methods, err := transport.BuildAuthForTest(transport.SSHConfig{
		User:       "ubuntu",
		Hostname:   "example.com",
		AuthMode:   transport.AuthAuto,
		Password:   "secret",
		PromptAuth: false,
	})
	require.NoError(t, err)
	require.Len(t, methods, 1)
}

func TestResolveAuthSelectionExplicitAutoUsesPassword(t *testing.T) {
	sel, err := transport.ResolveAuthSelection(transport.SSHConfig{
		User:     "ubuntu",
		Hostname: "example.com",
		AuthMode: transport.AuthAuto,
	}, true)
	require.NoError(t, err)
	require.Equal(t, transport.AuthPassword, sel.Mode)
	require.Empty(t, sel.IdentityFile)
}

func TestResolveAuthSelectionIdentityFileImpliesKey(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "vps_key")
	require.NoError(t, os.WriteFile(keyPath, []byte("key"), 0600))

	sel, err := transport.ResolveAuthSelection(transport.SSHConfig{
		User:         "ubuntu",
		Hostname:     "example.com",
		IdentityFile: keyPath,
	}, false)
	require.NoError(t, err)
	require.Equal(t, transport.AuthKey, sel.Mode)
	require.Equal(t, keyPath, sel.IdentityFile)
}

func TestResolveAuthSelectionPromptsPassword(t *testing.T) {
	restore := transport.SetInteractiveForTest(func() bool { return true })
	defer restore()

	r, w, err := os.Pipe()
	require.NoError(t, err)
	oldStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = oldStdin
		_ = r.Close()
	})
	_, _ = w.WriteString("1\n")
	_ = w.Close()

	sel, err := transport.ResolveAuthSelection(transport.SSHConfig{
		User:     "ubuntu",
		Hostname: "example.com",
	}, false)
	require.NoError(t, err)
	require.Equal(t, transport.AuthPassword, sel.Mode)
}

func TestResolveAuthSelectionNonInteractiveRequiresFlag(t *testing.T) {
	restore := transport.SetInteractiveForTest(func() bool { return false })
	defer restore()

	_, err := transport.ResolveAuthSelection(transport.SSHConfig{
		User:     "ubuntu",
		Hostname: "example.com",
	}, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "SSH auth not specified")
}

func TestParseAuthMode(t *testing.T) {
	mode, err := transport.ParseAuthMode("password")
	require.NoError(t, err)
	require.Equal(t, transport.AuthPassword, mode)
}
