package cli_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/degoke/outpost/internal/cli"
	"github.com/degoke/outpost/internal/mirror"
	"github.com/degoke/outpost/internal/output"
	"github.com/stretchr/testify/require"
)

func TestFinishRunResultJSONIncludesExitCode(t *testing.T) {
	var buf bytes.Buffer
	app := &cli.App{Out: output.New(true, false)}
	app.Out.Stdout = &buf

	code, err := app.FinishRunResultForTest(mirror.RunResult{
		ExitCode:    mirror.ExitSessionDetached,
		SessionName: "batch1",
		Detached:    true,
	}, nil)
	require.NoError(t, err)
	require.Equal(t, mirror.ExitSessionDetached, code)

	var payload mirror.RunResult
	require.NoError(t, json.Unmarshal(buf.Bytes(), &payload))
	require.Equal(t, mirror.ExitSessionDetached, payload.ExitCode)
	require.Equal(t, "batch1", payload.SessionName)
	require.True(t, payload.Detached)
}
