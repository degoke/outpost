package prune_test

import (
	"context"
	"testing"

	"github.com/degoke/outpost/internal/prune"
	"github.com/degoke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

func TestBuildPlanClustersIncludesK3d(t *testing.T) {
	exec := mock.New()
	exec.Responses["kind get clusters 2>/dev/null || true"] = struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{Stdout: "outpost-dev\n", ExitCode: 0}
	exec.Responses["k3d cluster list 2>/dev/null | awk 'NR>1 && NF {print $1}' || true"] = struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{Stdout: "outpost-staging\n", ExitCode: 0}

	plan, err := prune.BuildPlan(context.Background(), exec, prune.Options{Clusters: true})
	require.NoError(t, err)
	require.Len(t, plan.Candidates, 2)

	drivers := map[string]string{}
	for _, c := range plan.Candidates {
		drivers[c.ID] = c.Driver
	}
	require.Equal(t, "kind", drivers["outpost-dev"])
	require.Equal(t, "k3d", drivers["outpost-staging"])
}

func TestExecuteClustersDeletesK3d(t *testing.T) {
	exec := mock.New()
	exec.Responses["k3d cluster delete"] = struct {
		Stdout, Stderr string
		ExitCode       int
		Err            error
	}{ExitCode: 0}

	plan := &prune.PrunePlan{
		Candidates: []prune.Candidate{
			{Kind: "cluster", ID: "outpost-staging", Driver: "k3d"},
		},
	}
	result, err := prune.Execute(context.Background(), exec, plan, prune.Options{Clusters: true})
	require.NoError(t, err)
	require.Len(t, result.Removed, 1)
	require.True(t, exec.HasCommand("k3d cluster delete"))
}
