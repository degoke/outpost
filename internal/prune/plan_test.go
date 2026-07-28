package prune_test

import (
	"context"
	"testing"

	"github.com/degoke/outpost/internal/prune"
	"github.com/degoke/outpost/internal/transport/mock"
	"github.com/stretchr/testify/require"
)

func TestBuildPlanProtectsRunningCompose(t *testing.T) {
	exec := mock.New()
	exec.Responses["docker ps -a --format"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{
		Stdout: `{"ID":"abc","Names":"web-1","Image":"nginx","State":"exited","Status":"Exited","Labels":"com.docker.compose.project=demo"}`,
	}
	exec.Responses["docker compose ls"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: `[{"Name":"demo","Status":"running(1)"}]`}
	exec.Responses["docker ps  --format"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{
		Stdout: `{"ID":"run1","Names":"api-1","Image":"api","State":"running","Status":"Up","Labels":"com.docker.compose.project=demo"}`,
	}

	plan, err := prune.BuildPlan(context.Background(), exec, prune.Options{})
	require.NoError(t, err)
	for _, c := range plan.Candidates {
		if c.Kind == "container" && c.Name == "web-1" {
			t.Fatalf("stopped container in active compose project should be protected")
		}
	}
}

func TestBuildPlanVolumeDryRun(t *testing.T) {
	exec := mock.New()
	exec.Responses["docker volume ls -q"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: "data-vol\n"}
	exec.Responses["docker volume inspect"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{
		Stdout: `[{"Name":"data-vol","Driver":"local","Mountpoint":"/var/lib/docker/volumes/data-vol/_data","RefCount":0}]`,
	}

	plan, err := prune.BuildPlan(context.Background(), exec, prune.Options{Volumes: true})
	require.NoError(t, err)
	require.Len(t, plan.Candidates, 1)
	require.Equal(t, "volume", plan.Candidates[0].Kind)
}

func TestBuildPlanIncludesUnreferencedDanglingImage(t *testing.T) {
	exec := mock.New()
	exec.Responses["docker ps -a --format"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{
		Stdout: `{"ID":"run1","Names":"web-1","Image":"nginx:latest","State":"running","Status":"Up","Labels":""}`,
	}
	exec.Responses["docker ps  --format"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{
		Stdout: `{"ID":"run1","Names":"web-1","Image":"nginx:latest","State":"running","Status":"Up","Labels":""}`,
	}
	exec.Responses["docker images --format"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{
		Stdout: `{"Repository":"<none>","Tag":"<none>","ID":"sha256:dead","Size":"100MB"}`,
	}
	exec.Responses["find "] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: ""}

	plan, err := prune.BuildPlan(context.Background(), exec, prune.Options{})
	require.NoError(t, err)
	found := false
	for _, c := range plan.Candidates {
		if c.Kind == "image" && c.ID == "sha256:dead" {
			found = true
		}
	}
	require.True(t, found)
}

func TestBuildPlanSkipsReferencedImageID(t *testing.T) {
	exec := mock.New()
	exec.Responses["docker ps -a --format"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{
		Stdout: `{"ID":"run1","Names":"web-1","Image":"sha256:abc","State":"running","Status":"Up","Labels":""}`,
	}
	exec.Responses["docker ps  --format"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{
		Stdout: `{"ID":"run1","Names":"web-1","Image":"sha256:abc","State":"running","Status":"Up","Labels":""}`,
	}
	exec.Responses["docker images --format"] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{
		Stdout: `{"Repository":"<none>","Tag":"<none>","ID":"sha256:abc","Size":"100MB"}`,
	}
	exec.Responses["find "] = struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{Stdout: ""}

	plan, err := prune.BuildPlan(context.Background(), exec, prune.Options{})
	require.NoError(t, err)
	for _, c := range plan.Candidates {
		if c.Kind == "image" {
			t.Fatalf("in-use image should be protected")
		}
	}
}
