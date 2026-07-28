package prune_test

import (
	"context"
	"testing"

	"github.com/degoke/outpost/internal/prune"
	"github.com/degoke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

func TestBuildPlanMachines(t *testing.T) {
	exec := mock.New()
	exec.Responses["incus list --format json 2>/dev/null || true"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{
		Stdout:   `[{"name":"outpost-old","status":"Stopped","type":"container","state":{}}]`,
		ExitCode: 0,
	}

	plan, err := prune.BuildPlan(context.Background(), exec, prune.Options{Machines: true})
	require.NoError(t, err)
	require.Len(t, plan.Candidates, 1)
	require.Equal(t, "machine", plan.Candidates[0].Kind)
	require.Equal(t, "outpost-old", plan.Candidates[0].ID)
	require.Equal(t, "old", plan.Candidates[0].Name)
}

func TestExecuteMachinesRemovesMetadata(t *testing.T) {
	exec := mock.New()
	plan := &prune.PrunePlan{
		Candidates: []prune.Candidate{{
			Kind: "machine", ID: "outpost-old", Name: "old",
		}},
	}
	result, err := prune.Execute(context.Background(), exec, plan, prune.Options{Machines: true})
	require.NoError(t, err)
	require.Len(t, result.Removed, 1)
	require.True(t, exec.HasCommand("incus delete"))
	require.True(t, exec.HasCommand("rm -rf '/var/lib/outpost/machines/old'"))
}
