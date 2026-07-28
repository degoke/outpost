package capabilities_test

import (
	"context"
	"testing"

	"github.com/degoke/outpost/internal/capabilities"
	"github.com/degoke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

func TestDetectCapabilities(t *testing.T) {
	exec := mock.New()
	exec.Responses["docker info"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{}
	exec.Responses["docker compose version"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{}
	exec.Responses["curl -s"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: ""}

	report, err := capabilities.Detect(context.Background(), exec)
	require.NoError(t, err)
	require.NotEmpty(t, report.Supported)
	require.NotEmpty(t, report.Unavailable)
	names := map[string]bool{}
	unavail := map[string]bool{}
	for _, c := range report.Supported {
		names[c.Name] = true
	}
	for _, c := range report.Unavailable {
		unavail[c.Name] = true
	}
	require.True(t, names["docker"])
	require.True(t, names["compose"])
	require.True(t, unavail["vm"] || names["vm"])
}
