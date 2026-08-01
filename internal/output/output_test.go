package output_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/degoke/outpost/internal/output"
	"github.com/stretchr/testify/require"
)

func TestSuccessRendersPlainWhenNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var buf bytes.Buffer
	p := output.New(false, false)
	p.Stdout = &buf
	p.Success("Initialized project %q", "demo")
	require.Equal(t, "✓ Initialized project \"demo\"\n", buf.String())
}

func TestStepRendersArrowPrefix(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var buf bytes.Buffer
	p := output.New(false, false)
	p.Stderr = &buf
	p.Step("Syncing repository...")
	require.Equal(t, "→ Syncing repository...\n", buf.String())
}

func TestJSONModeOmitsStyledOutput(t *testing.T) {
	var stdout bytes.Buffer
	p := output.New(true, false)
	p.Stdout = &stdout
	p.Success("ignored")
	p.Step("ignored")
	require.Empty(t, stdout.String())
}

func TestColorDisabledForNonTTYWriter(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	var buf bytes.Buffer
	p := output.New(false, false)
	p.Stdout = &buf
	p.Success("ready")
	require.True(t, strings.HasPrefix(buf.String(), "✓ "))
	require.NotContains(t, buf.String(), "\x1b[")
}

func TestErrorUsesPlainPrefixInJSONMode(t *testing.T) {
	var stderr bytes.Buffer
	p := output.New(true, false)
	p.Stderr = &stderr
	p.Error("boom")
	require.Equal(t, "error: boom\n", stderr.String())
}

func TestInfoSkipsInJSONMode(t *testing.T) {
	var stdout bytes.Buffer
	p := output.New(true, false)
	p.Stdout = &stdout
	p.Info("line")
	require.Empty(t, stdout.String())
}

func TestColorEnabledRespectsNoColorEnv(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var buf bytes.Buffer
	p := output.New(false, false)
	p.Stdout = os.Stdout
	p.Stderr = &buf
	p.Step("working")
	require.Equal(t, "→ working\n", buf.String())
}
