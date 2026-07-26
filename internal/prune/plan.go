package prune

import (
	"context"
	"fmt"
	"strings"

	"github.com/goke/outpost/internal/inspect"
	"github.com/goke/outpost/internal/transport"
)

type Candidate struct {
	Kind           string `json:"kind"`
	ID             string `json:"id"`
	Name           string `json:"name,omitempty"`
	EstimatedBytes int64  `json:"estimated_bytes,omitempty"`
	Reason         string `json:"reason,omitempty"`
}

type PrunePlan struct {
	Candidates     []Candidate `json:"candidates"`
	EstimatedBytes int64       `json:"estimated_bytes"`
	ProtectedNote  string      `json:"protected_note,omitempty"`
}

type Options struct {
	Volumes  bool
	Clusters bool
	Force    bool
}

type Result struct {
	Removed        []Candidate `json:"removed"`
	EstimatedBytes int64       `json:"estimated_bytes"`
	ReclaimedBytes int64       `json:"reclaimed_bytes,omitempty"`
}

func BuildPlan(ctx context.Context, exec transport.Executor, opts Options) (*PrunePlan, error) {
	plan := &PrunePlan{
		ProtectedNote: "named volumes, running containers, active compose project images, and in-use images are protected by default",
	}
	runningProjects, _ := inspect.RunningComposeProjects(ctx, exec)
	imagesInUse, _ := inspect.ImagesInUse(ctx, exec)

	if opts.Volumes {
		vols, err := inspect.ListVolumes(ctx, exec)
		if err != nil {
			return nil, err
		}
		for _, v := range vols {
			if v.InUse {
				continue
			}
			plan.Candidates = append(plan.Candidates, Candidate{
				Kind: "volume", ID: v.Name, Name: v.Name,
				Reason: "unused named volume",
			})
		}
		return plan, nil
	}

	if opts.Clusters {
		clusters, err := inspect.RunOutput(ctx, exec, "kind get clusters 2>/dev/null || true")
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(strings.TrimSpace(clusters), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			plan.Candidates = append(plan.Candidates, Candidate{
				Kind: "cluster", ID: line, Name: strings.TrimPrefix(line, "outpost-"),
				Reason: "kind cluster",
			})
		}
		return plan, nil
	}

	containers, err := inspect.ListContainers(ctx, exec, true)
	if err != nil {
		return nil, err
	}
	for _, c := range containers {
		if strings.EqualFold(c.State, "running") {
			continue
		}
		if c.Project != "" && runningProjects[c.Project] {
			continue
		}
		plan.Candidates = append(plan.Candidates, Candidate{
			Kind: "container", ID: c.ID, Name: c.Name,
			Reason: "stopped container",
		})
	}

	images, err := inspect.ListImages(ctx, exec)
	if err == nil {
		for _, img := range images {
			if !img.Dangling {
				continue
			}
			if imageInUse(img, imagesInUse) {
				continue
			}
			plan.Candidates = append(plan.Candidates, Candidate{
				Kind: "image", ID: img.ID, Name: img.RepoTags,
				EstimatedBytes: img.Size, Reason: "dangling image",
			})
			plan.EstimatedBytes += img.Size
		}
	}

	uploadFiles, _ := inspect.ListUploadTempFiles(ctx, exec)
	for _, f := range uploadFiles {
		plan.Candidates = append(plan.Candidates, Candidate{
			Kind: "upload_temp", ID: f, Name: f, Reason: "stale upload artifact",
		})
	}

	plan.Candidates = append(plan.Candidates, Candidate{
		Kind: "network", ID: "(unused)", Reason: "unused networks",
	})
	plan.Candidates = append(plan.Candidates, Candidate{
		Kind: "build_cache", ID: "(cache)", Reason: "old build cache",
	})

	return plan, nil
}

func imageInUse(img inspect.Image, inUse map[string]bool) bool {
	if inUse[img.ID] || inUse[img.RepoTags] {
		return true
	}
	if len(img.ID) > 12 && inUse[img.ID[:12]] {
		return true
	}
	return false
}

func Execute(ctx context.Context, exec transport.Executor, plan *PrunePlan, opts Options) (*Result, error) {
	if opts.Volumes {
		return executeVolumes(ctx, exec, plan)
	}
	if opts.Clusters {
		return executeClusters(ctx, exec, plan)
	}
	before, _ := inspect.DockerReclaimableBytes(ctx, exec)
	result := &Result{EstimatedBytes: plan.EstimatedBytes}
	seen := map[string]bool{}
	for _, c := range plan.Candidates {
		var cmd string
		switch c.Kind {
		case "container":
			cmd = fmt.Sprintf("docker rm %s", shellQuote(c.ID))
		case "image":
			cmd = fmt.Sprintf("docker rmi %s", shellQuote(c.ID))
		case "upload_temp":
			cmd = fmt.Sprintf("rm -f %s", shellQuote(c.ID))
		case "network":
			if seen["network"] {
				continue
			}
			seen["network"] = true
			cmd = "docker network prune -f"
		case "build_cache":
			if seen["build_cache"] {
				continue
			}
			seen["build_cache"] = true
			cmd = "docker builder prune -f --filter until=24h"
		default:
			continue
		}
		code, err := exec.Run(ctx, cmd, transport.RunOpts{})
		if err != nil {
			return result, err
		}
		if code == 0 {
			result.Removed = append(result.Removed, c)
		}
	}
	after, _ := inspect.DockerReclaimableBytes(ctx, exec)
	if before > after {
		result.ReclaimedBytes = before - after
	}
	return result, nil
}

func executeVolumes(ctx context.Context, exec transport.Executor, plan *PrunePlan) (*Result, error) {
	result := &Result{EstimatedBytes: plan.EstimatedBytes}
	for _, c := range plan.Candidates {
		if c.Kind != "volume" {
			continue
		}
		cmd := fmt.Sprintf("docker volume rm %s", shellQuote(c.ID))
		code, err := exec.Run(ctx, cmd, transport.RunOpts{})
		if err != nil {
			return result, err
		}
		if code == 0 {
			result.Removed = append(result.Removed, c)
		}
	}
	return result, nil
}

func executeClusters(ctx context.Context, exec transport.Executor, plan *PrunePlan) (*Result, error) {
	result := &Result{EstimatedBytes: plan.EstimatedBytes}
	for _, c := range plan.Candidates {
		if c.Kind != "cluster" {
			continue
		}
		cmd := fmt.Sprintf("kind delete cluster --name %s", shellQuote(c.ID))
		code, err := exec.Run(ctx, cmd, transport.RunOpts{})
		if err != nil {
			return result, err
		}
		if code == 0 {
			result.Removed = append(result.Removed, c)
		}
	}
	return result, nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
