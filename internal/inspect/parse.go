package inspect

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func ParseNproc(out string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil || n < 1 {
		return 1, fmt.Errorf("parse nproc: %q", out)
	}
	return n, nil
}

func ParseFreeBytes(out string) (total, used, available uint64, err error) {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, "Mem:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 7 {
			break
		}
		total, _ = strconv.ParseUint(fields[1], 10, 64)
		used, _ = strconv.ParseUint(fields[2], 10, 64)
		available, _ = strconv.ParseUint(fields[6], 10, 64)
		return total, used, available, nil
	}
	return 0, 0, 0, fmt.Errorf("parse free output")
}

func ParseDFBytes(out string) (total, used, avail uint64, err error) {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		return 0, 0, 0, fmt.Errorf("parse df output")
	}
	fields := strings.Fields(lines[1])
	if len(fields) < 4 {
		return 0, 0, 0, fmt.Errorf("parse df fields")
	}
	total, _ = strconv.ParseUint(fields[1], 10, 64)
	used, _ = strconv.ParseUint(fields[2], 10, 64)
	avail, _ = strconv.ParseUint(fields[3], 10, 64)
	return total, used, avail, nil
}

func ParseUptimeSeconds(out string) (uint64, error) {
	fields := strings.Fields(strings.TrimSpace(out))
	if len(fields) < 1 {
		return 0, fmt.Errorf("parse uptime")
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, err
	}
	return uint64(v), nil
}

func ParseProcStatCPU(out string) (idle, total uint64, err error) {
	line := strings.TrimSpace(out)
	if !strings.HasPrefix(line, "cpu ") {
		return 0, 0, fmt.Errorf("unexpected /proc/stat line")
	}
	fields := strings.Fields(line)
	if len(fields) < 5 {
		return 0, 0, fmt.Errorf("short /proc/stat line")
	}
	var vals []uint64
	for _, f := range fields[1:] {
		v, err := strconv.ParseUint(f, 10, 64)
		if err != nil {
			return 0, 0, err
		}
		vals = append(vals, v)
	}
	for _, v := range vals {
		total += v
	}
	idle = vals[3]
	if len(vals) > 4 {
		idle += vals[4]
	}
	return idle, total, nil
}

func CPUUsagePercent(idle1, total1, idle2, total2 uint64) float64 {
	idleDelta := float64(idle2 - idle1)
	totalDelta := float64(total2 - total1)
	if totalDelta <= 0 {
		return 0
	}
	return (1.0 - idleDelta/totalDelta) * 100
}

func ParseDockerPSLines(out string) ([]Container, error) {
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	var containers []Container
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row struct {
			ID     string `json:"ID"`
			Names  string `json:"Names"`
			Image  string `json:"Image"`
			State  string `json:"State"`
			Status string `json:"Status"`
			Labels string `json:"Labels"`
		}
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			continue
		}
		c := Container{
			ID: row.ID, Name: row.Names, Image: row.Image,
			State: row.State, Status: row.Status,
		}
		c.Project = composeProjectFromLabels(row.Labels)
		containers = append(containers, c)
	}
	return containers, nil
}

func composeProjectFromLabels(labels string) string {
	for _, part := range strings.Split(labels, ",") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "com.docker.compose.project=") {
			return strings.TrimPrefix(part, "com.docker.compose.project=")
		}
	}
	return ""
}

func ParseDockerStatsLines(out string) ([]ContainerStats, error) {
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	var stats []ContainerStats
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row struct {
			ID          string `json:"ID"`
			Name        string `json:"Name"`
			CPUPerc     string `json:"CPUPerc"`
			MemUsage    string `json:"MemUsage"`
			MemPerc     string `json:"MemPerc"`
			MemLimit    string `json:"MemLimit"`
			BlockIO     string `json:"BlockIO"`
			NetIO       string `json:"NetIO"`
			PIDs        string `json:"PIDs"`
			Container   string `json:"Container"`
			BlockLimit  string `json:"BlockLimit"`
			NetLimit    string `json:"NetLimit"`
		}
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			continue
		}
		cpu, _ := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(row.CPUPerc), "%"), 64)
		memUsed, memLimit := parseMemUsage(row.MemUsage)
		memPct, _ := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(row.MemPerc), "%"), 64)
		stats = append(stats, ContainerStats{
			ID: row.ID, Name: strings.TrimPrefix(row.Name, "/"),
			CPUPercent: cpu, MemUsage: memUsed, MemLimit: memLimit, MemPercent: memPct,
		})
	}
	return stats, nil
}

func parseMemUsage(s string) (used, limit uint64) {
	parts := strings.Split(s, "/")
	if len(parts) != 2 {
		return 0, 0
	}
	used = parseByteQuantity(strings.TrimSpace(parts[0]))
	limit = parseByteQuantity(strings.TrimSpace(parts[1]))
	return used, limit
}

func parseByteQuantity(s string) uint64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	mult := uint64(1)
	switch {
	case strings.HasSuffix(s, "GiB"):
		mult = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "GiB")
	case strings.HasSuffix(s, "MiB"):
		mult = 1024 * 1024
		s = strings.TrimSuffix(s, "MiB")
	case strings.HasSuffix(s, "KiB"):
		mult = 1024
		s = strings.TrimSuffix(s, "KiB")
	case strings.HasSuffix(s, "GB"):
		mult = 1000 * 1000 * 1000
		s = strings.TrimSuffix(s, "GB")
	case strings.HasSuffix(s, "MB"):
		mult = 1000 * 1000
		s = strings.TrimSuffix(s, "MB")
	case strings.HasSuffix(s, "kB"):
		mult = 1000
		s = strings.TrimSuffix(s, "kB")
	case strings.HasSuffix(s, "B"):
		s = strings.TrimSuffix(s, "B")
	}
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return uint64(v * float64(mult))
}

func ParseDockerSystemDF(out string) ([]DockerDiskRow, error) {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		return nil, nil
	}
	var rows []DockerDiskRow
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		total, _ := strconv.ParseInt(fields[1], 10, 64)
		active, _ := strconv.ParseInt(fields[2], 10, 64)
		reclaimable := ""
		if len(fields) >= 5 {
			reclaimable = fields[4]
		}
		rows = append(rows, DockerDiskRow{
			Type: fields[0], Total: total, Active: active,
			Size: fields[3], Reclaimable: reclaimable,
		})
	}
	return rows, nil
}

func ParseComposeLS(out string) ([]ComposeProject, error) {
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	var projects []ComposeProject
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row struct {
			Name   string `json:"Name"`
			Status string `json:"Status"`
		}
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			continue
		}
		projects = append(projects, ComposeProject{Name: row.Name, Status: row.Status})
	}
	return projects, nil
}

func ParseDuLine(out string) (uint64, error) {
	fields := strings.Fields(strings.TrimSpace(out))
	if len(fields) < 1 {
		return 0, fmt.Errorf("parse du")
	}
	v, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return 0, err
	}
	return v, nil
}

func ParseDockerVolumesJSON(out string) ([]Volume, error) {
	out = strings.TrimSpace(out)
	if out == "" || out == "[]" {
		return nil, nil
	}
	var rows []struct {
		Name       string `json:"Name"`
		Driver     string `json:"Driver"`
		Mountpoint string `json:"Mountpoint"`
		RefCount   int    `json:"RefCount"`
	}
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		return nil, err
	}
	var vols []Volume
	for _, r := range rows {
		vols = append(vols, Volume{
			Name: r.Name, Driver: r.Driver, Mountpoint: r.Mountpoint,
			RefCount: r.RefCount, InUse: r.RefCount > 0,
		})
	}
	return vols, nil
}

func ParseDockerImagesLines(out string) ([]Image, error) {
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	var images []Image
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row struct {
			ID         string `json:"ID"`
			Repository string `json:"Repository"`
			Tag        string `json:"Tag"`
			Size       string `json:"Size"`
		}
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			continue
		}
		repoTags := row.Repository
		if row.Tag != "" && row.Tag != "<none>" {
			repoTags = row.Repository + ":" + row.Tag
		}
		dangling := row.Repository == "<none>" || row.Repository == ""
		images = append(images, Image{
			ID: row.ID, RepoTags: repoTags, Size: ParseSizeToBytes(row.Size), Dangling: dangling,
		})
	}
	return images, nil
}

func ParseSizeToBytes(s string) int64 {
	return int64(parseByteQuantity(strings.TrimSpace(s)))
}

func SumReclaimableBytes(rows []DockerDiskRow) int64 {
	var total int64
	for _, row := range rows {
		if row.Reclaimable == "" {
			continue
		}
		part := strings.TrimSpace(strings.Split(row.Reclaimable, "(")[0])
		total += ParseSizeToBytes(part)
	}
	return total
}
