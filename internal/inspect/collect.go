package inspect

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/goke/outpost/internal/config"
	"github.com/goke/outpost/internal/transport"
)

func CollectHostMetrics(ctx context.Context, exec transport.Executor) (HostMetrics, error) {
	var m HostMetrics
	var out bytes.Buffer
	code, err := exec.Run(ctx, "nproc", transport.RunOpts{Stdout: &out})
	if err != nil {
		return m, err
	}
	if code == 0 {
		m.CPUCores, _ = ParseNproc(out.String())
	}

	out.Reset()
	code, err = exec.Run(ctx, "free -b | head -2", transport.RunOpts{Stdout: &out})
	if err == nil && code == 0 {
		total, used, avail, err := ParseFreeBytes(out.String())
		if err == nil {
			m.MemoryTotal, m.MemoryUsed, m.MemoryAvailable = total, used, avail
		}
	}

	out.Reset()
	code, err = exec.Run(ctx, "df -B1 / | tail -1", transport.RunOpts{Stdout: &out})
	if err == nil && code == 0 {
		total, used, avail, err := ParseDFBytes("Filesystem\n" + strings.TrimSpace(out.String()))
		if err == nil {
			m.DiskTotal, m.DiskUsed, m.DiskAvailable = total, used, avail
		}
	}

	out.Reset()
	code, err = exec.Run(ctx, "cat /proc/uptime", transport.RunOpts{Stdout: &out})
	if err == nil && code == 0 {
		m.UptimeSeconds, _ = ParseUptimeSeconds(out.String())
	}

	idle1, total1, err := readProcStat(ctx, exec)
	if err == nil {
		time.Sleep(200 * time.Millisecond)
		idle2, total2, err := readProcStat(ctx, exec)
		if err == nil {
			m.CPUUsagePercent = CPUUsagePercent(idle1, total1, idle2, total2)
		}
	}
	return m, nil
}

func readProcStat(ctx context.Context, exec transport.Executor) (idle, total uint64, err error) {
	var out bytes.Buffer
	code, err := exec.Run(ctx, "head -1 /proc/stat", transport.RunOpts{Stdout: &out})
	if err != nil || code != 0 {
		return 0, 0, fmt.Errorf("read /proc/stat")
	}
	return ParseProcStatCPU(out.String())
}

func CollectDockerSummary(ctx context.Context, exec transport.Executor) (DockerSummary, error) {
	var s DockerSummary
	code, err := exec.Run(ctx, "docker info >/dev/null 2>&1", transport.RunOpts{})
	if err != nil {
		return s, err
	}
	s.Healthy = code == 0
	if !s.Healthy {
		return s, nil
	}

	containers, err := ListContainers(ctx, exec, true)
	if err == nil {
		s.ContainersTotal = len(containers)
		for _, c := range containers {
			if strings.EqualFold(c.State, "running") {
				s.ContainersRun++
			} else {
				s.ContainersStop++
			}
		}
	}

	var out bytes.Buffer
	code, err = exec.Run(ctx, "docker images -q | wc -l", transport.RunOpts{Stdout: &out})
	if err == nil && code == 0 {
		fmt.Sscanf(strings.TrimSpace(out.String()), "%d", &s.Images)
	}

	out.Reset()
	code, err = exec.Run(ctx, "docker volume ls -q | wc -l", transport.RunOpts{Stdout: &out})
	if err == nil && code == 0 {
		fmt.Sscanf(strings.TrimSpace(out.String()), "%d", &s.Volumes)
	}

	out.Reset()
	code, err = exec.Run(ctx, "docker system df", transport.RunOpts{Stdout: &out})
	if err == nil && code == 0 {
		s.DiskUsage, _ = ParseDockerSystemDF(out.String())
	}
	return s, nil
}

func ListContainers(ctx context.Context, exec transport.Executor, all bool) ([]Container, error) {
	flag := ""
	if all {
		flag = "-a"
	}
	var out bytes.Buffer
	cmd := fmt.Sprintf("docker ps %s --format '{{json .}}'", flag)
	code, err := exec.Run(ctx, cmd, transport.RunOpts{Stdout: &out})
	if err != nil || code != 0 {
		return nil, fmt.Errorf("docker ps failed")
	}
	return ParseDockerPSLines(out.String())
}

func ListContainerStats(ctx context.Context, exec transport.Executor) ([]ContainerStats, error) {
	var out bytes.Buffer
	cmd := "docker stats --no-stream --format '{{json .}}'"
	code, err := exec.Run(ctx, cmd, transport.RunOpts{Stdout: &out})
	if err != nil || code != 0 {
		return nil, fmt.Errorf("docker stats failed")
	}
	stats, err := ParseDockerStatsLines(out.String())
	if err != nil {
		return nil, err
	}
	containers, _ := ListContainers(ctx, exec, false)
	byName := map[string]string{}
	for _, c := range containers {
		byName[c.Name] = c.Project
	}
	for i := range stats {
		if p, ok := byName[stats[i].Name]; ok {
			stats[i].Project = p
		}
	}
	return stats, nil
}

func ListKindNodeStats(ctx context.Context, exec transport.Executor) ([]ContainerStats, error) {
	var out bytes.Buffer
	cmd := "docker stats --no-stream --filter label=io.x-k8s.kind.role --format '{{json .}}'"
	code, err := exec.Run(ctx, cmd, transport.RunOpts{Stdout: &out})
	if err != nil || code != 0 {
		return nil, nil
	}
	stats, err := ParseDockerStatsLines(out.String())
	if err != nil {
		return nil, err
	}
	for i := range stats {
		stats[i].Project = "kind:" + stats[i].Name
	}
	return stats, nil
}

func ListComposeProjects(ctx context.Context, exec transport.Executor) ([]ComposeProject, error) {
	var out bytes.Buffer
	code, err := exec.Run(ctx, "docker compose ls --format json", transport.RunOpts{Stdout: &out})
	if err != nil || code != 0 {
		return nil, nil
	}
	return ParseComposeLS(out.String())
}

func ListIncusInstanceStats(ctx context.Context, exec transport.Executor) ([]ContainerStats, error) {
	out, err := RunOutput(ctx, exec, "incus list --format json 2>/dev/null || true")
	if err != nil {
		return nil, nil
	}
	out = strings.TrimSpace(out)
	if out == "" || out == "[]" {
		return nil, nil
	}
	var entries []struct {
		Name  string `json:"name"`
		Type  string `json:"type"`
		State struct {
			Memory struct {
				Usage uint64 `json:"usage"`
			} `json:"memory"`
			Processes int `json:"processes"`
		} `json:"state"`
	}
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		return nil, nil
	}
	var stats []ContainerStats
	for _, e := range entries {
		if !strings.HasPrefix(e.Name, "outpost-") {
			continue
		}
		kind := "container"
		if strings.EqualFold(e.Type, "virtual-machine") {
			kind = "vm"
		}
		stats = append(stats, ContainerStats{
			Name:       e.Name,
			MemUsage:   e.State.Memory.Usage,
			MemLimit:   e.State.Memory.Usage,
			MemPercent: 0,
			Project:    "machine:" + kind,
		})
	}
	return stats, nil
}

func CollectOutpostDirs(ctx context.Context, exec transport.Executor) (OutpostDirs, error) {
	var d OutpostDirs
	var out bytes.Buffer
	cmd := fmt.Sprintf("du -sb %s/projects %s/share %s/machines 2>/dev/null || true", config.DefaultRemoteBase, config.DefaultRemoteBase, config.DefaultRemoteBase)
	code, err := exec.Run(ctx, cmd, transport.RunOpts{Stdout: &out})
	if err != nil || code != 0 {
		return d, nil
	}
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		v, err := ParseDuLine(line)
		if err != nil {
			continue
		}
		if strings.Contains(line, "/projects") {
			d.ProjectsBytes = v
		} else if strings.Contains(line, "/share") {
			d.ShareBytes = v
		} else if strings.Contains(line, "/machines") {
			d.MachinesBytes = v
		}
	}
	return d, nil
}

func ListVolumes(ctx context.Context, exec transport.Executor) ([]Volume, error) {
	var out bytes.Buffer
	code, err := exec.Run(ctx, "docker volume ls -q", transport.RunOpts{Stdout: &out})
	if err != nil || code != 0 {
		return nil, fmt.Errorf("docker volume ls failed")
	}
	names := strings.Fields(strings.TrimSpace(out.String()))
	if len(names) == 0 {
		return nil, nil
	}
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = "'" + strings.ReplaceAll(n, "'", "'\\''") + "'"
	}
	cmd := fmt.Sprintf("docker volume inspect %s", strings.Join(quoted, " "))
	out.Reset()
	code, err = exec.Run(ctx, cmd, transport.RunOpts{Stdout: &out})
	if err != nil || code != 0 {
		return nil, fmt.Errorf("docker volume inspect failed")
	}
	return ParseDockerVolumesJSON(out.String())
}

func RunningComposeProjects(ctx context.Context, exec transport.Executor) (map[string]bool, error) {
	projects := map[string]bool{}
	containers, err := ListContainers(ctx, exec, false)
	if err != nil {
		return projects, err
	}
	for _, c := range containers {
		if c.Project != "" {
			projects[c.Project] = true
		}
	}
	return projects, nil
}

func RunningContainerIDs(ctx context.Context, exec transport.Executor) (map[string]bool, error) {
	ids := map[string]bool{}
	containers, err := ListContainers(ctx, exec, false)
	if err != nil {
		return ids, err
	}
	for _, c := range containers {
		ids[c.ID] = true
		if len(c.ID) > 12 {
			ids[c.ID[:12]] = true
		}
	}
	return ids, nil
}

func ListImages(ctx context.Context, exec transport.Executor) ([]Image, error) {
	var out bytes.Buffer
	cmd := `docker images --format '{{json .}}'`
	code, err := exec.Run(ctx, cmd, transport.RunOpts{Stdout: &out})
	if err != nil || code != 0 {
		return nil, fmt.Errorf("docker images failed")
	}
	return ParseDockerImagesLines(out.String())
}

func RunOutput(ctx context.Context, exec transport.Executor, cmd string) (string, error) {
	var out bytes.Buffer
	code, err := exec.Run(ctx, cmd, transport.RunOpts{Stdout: &out})
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("command failed (exit %d): %s", code, strings.TrimSpace(out.String()))
	}
	return out.String(), nil
}

func ImagesInUse(ctx context.Context, exec transport.Executor) (map[string]bool, error) {
	inUse := map[string]bool{}
	containers, err := ListContainers(ctx, exec, false)
	if err != nil {
		return inUse, err
	}
	for _, c := range containers {
		if c.Image == "" {
			continue
		}
		inUse[c.Image] = true
		parts := strings.Split(c.Image, ":")
		if len(parts) > 0 {
			inUse[parts[0]] = true
		}
		if len(c.Image) > 12 {
			inUse[c.Image[:12]] = true
		}
	}
	return inUse, nil
}

func DockerReclaimableBytes(ctx context.Context, exec transport.Executor) (int64, error) {
	var out bytes.Buffer
	code, err := exec.Run(ctx, "docker system df", transport.RunOpts{Stdout: &out})
	if err != nil || code != 0 {
		return 0, err
	}
	rows, err := ParseDockerSystemDF(out.String())
	if err != nil {
		return 0, err
	}
	return SumReclaimableBytes(rows), nil
}

func ListUploadTempFiles(ctx context.Context, exec transport.Executor) ([]string, error) {
	cmd := fmt.Sprintf(`find %s/projects -path '*/.upload-tmp/*' -type f 2>/dev/null || true`, config.DefaultRemoteBase)
	out, err := RunOutput(ctx, exec, cmd)
	if err != nil {
		return nil, nil
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

func CountUnusedNetworks(ctx context.Context, exec transport.Executor) (int, error) {
	var out bytes.Buffer
	code, err := exec.Run(ctx, "docker network ls --filter dangling=true -q | wc -l", transport.RunOpts{Stdout: &out})
	if err != nil || code != 0 {
		return 0, err
	}
	var n int
	fmt.Sscanf(strings.TrimSpace(out.String()), "%d", &n)
	return n, nil
}
