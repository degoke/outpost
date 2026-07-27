package inspect

type HostMetrics struct {
	CPUCores        int     `json:"cpu_cores"`
	CPUUsagePercent float64 `json:"cpu_usage_percent"`
	MemoryTotal     uint64  `json:"memory_total_bytes"`
	MemoryUsed      uint64  `json:"memory_used_bytes"`
	MemoryAvailable uint64  `json:"memory_available_bytes"`
	DiskTotal       uint64  `json:"disk_total_bytes"`
	DiskUsed        uint64  `json:"disk_used_bytes"`
	DiskAvailable   uint64  `json:"disk_available_bytes"`
	UptimeSeconds   uint64  `json:"uptime_seconds"`
}

type DockerSummary struct {
	Healthy         bool            `json:"healthy"`
	ContainersTotal int             `json:"containers_total"`
	ContainersRun   int             `json:"containers_running"`
	ContainersStop  int             `json:"containers_stopped"`
	Images          int             `json:"images"`
	Volumes         int             `json:"volumes"`
	DiskUsage       []DockerDiskRow `json:"disk_usage,omitempty"`
}

type DockerDiskRow struct {
	Type        string `json:"type"`
	Total       int64  `json:"total"`
	Active      int64  `json:"active"`
	Size        string `json:"size"`
	Reclaimable string `json:"reclaimable"`
}

type Container struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Image   string            `json:"image"`
	State   string            `json:"state"`
	Status  string            `json:"status"`
	Labels  map[string]string `json:"labels,omitempty"`
	Project string            `json:"compose_project,omitempty"`
}

type ContainerStats struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	CPUPercent float64 `json:"cpu_percent"`
	MemUsage   uint64  `json:"mem_usage_bytes"`
	MemLimit   uint64  `json:"mem_limit_bytes"`
	MemPercent float64 `json:"mem_percent"`
	Project    string  `json:"compose_project,omitempty"`
}

type ComposeProject struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type Volume struct {
	Name       string `json:"name"`
	Driver     string `json:"driver"`
	Mountpoint string `json:"mountpoint"`
	RefCount   int    `json:"ref_count"`
	SizeBytes  int64  `json:"size_bytes,omitempty"`
	InUse      bool   `json:"in_use"`
}

type OutpostDirs struct {
	ProjectsBytes uint64 `json:"projects_bytes"`
	ShareBytes    uint64 `json:"share_bytes"`
	MachinesBytes uint64 `json:"machines_bytes"`
}

type Image struct {
	ID       string `json:"id"`
	RepoTags string `json:"repo_tags"`
	Size     int64  `json:"size"`
	Dangling bool   `json:"dangling"`
	InUse    bool   `json:"in_use"`
}
