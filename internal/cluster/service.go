package cluster

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/degoke/outpost/internal/bootstrap"
	"github.com/degoke/outpost/internal/capacity"
	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/inspect"
	"github.com/degoke/outpost/internal/output"
	"github.com/degoke/outpost/internal/transport"
	"gopkg.in/yaml.v3"
)

type Cluster struct {
	Name          string `json:"name"`
	Driver        string `json:"driver"`
	KindName      string `json:"kind_name"`
	Workers       int    `json:"workers"`
	ControlPlanes int    `json:"control_planes"`
	Status        string `json:"status"`
	NodeCount     int    `json:"node_count"`
}

type Meta struct {
	Name          string    `yaml:"name"`
	Driver        string    `yaml:"driver,omitempty"`
	KindName      string    `yaml:"kind_name"`
	Workers       int       `yaml:"workers"`
	ControlPlanes int       `yaml:"control_planes"`
	CreatedAt     time.Time `yaml:"created_at"`
}

type Service struct {
	Exec     transport.Executor
	Out      *output.Printer
	HostName string
}

func (s *Service) Create(ctx context.Context, name string, driver Driver, workers, controlPlanes int) error {
	safe := config.SanitizeClusterName(name)
	if safe == "" {
		return fmt.Errorf("cluster name is required")
	}
	runtimeName := KindName(name)
	if controlPlanes == 0 {
		controlPlanes = 1
	}
	if err := bootstrap.Ensure(ctx, s.Exec); err != nil {
		return err
	}
	if err := bootstrap.EnsureKubernetesTools(ctx, s.Exec); err != nil {
		return err
	}
	cpu, mem, disk := EstimateResources(driver, controlPlanes, workers)
	if err := capacity.Check(ctx, s.Exec, capacity.Request{CPUCores: cpu, MemoryBytes: mem, DiskBytes: disk}); err != nil {
		return err
	}
	existing, _ := s.List(ctx)
	for _, c := range existing {
		if c.Name == safe {
			return fmt.Errorf("cluster %q already exists", name)
		}
	}

	remoteDir := RemoteDir(name)
	if err := transport.EnsureRemoteDir(s.Exec, remoteDir); err != nil {
		return err
	}
	cfgPath := remoteDir + "/kind-config.yaml"
	if driver == DriverKind {
		cfg := RenderKindConfig(KindConfig{Name: runtimeName, ControlPlanes: controlPlanes, Workers: workers})
		if err := s.Exec.UploadBytes([]byte(cfg), cfgPath); err != nil {
			return err
		}
	}

	driverLabel := driver.String()
	if s.Out != nil {
		s.Out.Step("Creating Kubernetes cluster %q with %s...", name, driverLabel)
	}
	code, err := createCluster(ctx, s.Exec, driver, runtimeName, workers, controlPlanes, cfgPath)
	if err != nil {
		_ = deleteCluster(ctx, s.Exec, driver, runtimeName)
		return err
	}
	if code != 0 {
		_ = deleteCluster(ctx, s.Exec, driver, runtimeName)
		return fmt.Errorf("%s cluster create failed (exit %d)", driverLabel, code)
	}

	if s.Out != nil {
		s.Out.Step("Saving kubeconfig...")
	}
	kubeOut, err := fetchKubeconfig(ctx, s.Exec, driver, runtimeName)
	if err != nil {
		_ = deleteCluster(ctx, s.Exec, driver, runtimeName)
		return fmt.Errorf("fetch kubeconfig: %w", err)
	}
	kubePath := RemoteKubeconfig(name)
	if err := s.Exec.UploadBytes([]byte(kubeOut), kubePath); err != nil {
		_ = deleteCluster(ctx, s.Exec, driver, runtimeName)
		return err
	}
	_, _ = s.Exec.Run(ctx, fmt.Sprintf("chmod 600 %s", shellQuote(kubePath)), transport.RunOpts{})

	meta := Meta{
		Name: safe, Driver: driver.String(), KindName: runtimeName,
		Workers: workers, ControlPlanes: controlPlanes, CreatedAt: time.Now().UTC(),
	}
	metaBytes, _ := yaml.Marshal(meta)
	if err := s.Exec.UploadBytes(metaBytes, remoteDir+"/meta.yaml"); err != nil {
		_ = deleteCluster(ctx, s.Exec, driver, runtimeName)
		return err
	}
	if err := s.syncLocalKubeconfig(name, kubeOut); err != nil {
		return err
	}
	if s.Out != nil && !s.Out.JSON {
		s.Out.Success("Cluster %q is ready (%s, %d control-plane, %d workers)", name, driverLabel, controlPlanes, workers)
	}
	return nil
}

func (s *Service) List(ctx context.Context) ([]Cluster, error) {
	runtimeClusters, err := listRuntimeClusters(ctx, s.Exec)
	if err != nil {
		return nil, err
	}
	metaClusters, _ := s.listMeta(ctx)
	var result []Cluster
	seen := map[string]bool{}
	for _, m := range metaClusters {
		drv := metaDriver(m)
		status := "unknown"
		if _, ok := runtimeClusters[m.KindName]; ok {
			status = "ready"
		}
		nodes, _ := nodeCountFor(ctx, s.Exec, drv, m.KindName)
		result = append(result, Cluster{
			Name: m.Name, Driver: drv.String(), KindName: m.KindName, Workers: m.Workers,
			ControlPlanes: m.ControlPlanes, Status: status, NodeCount: nodes,
		})
		seen[m.Name] = true
	}
	for rn, drv := range runtimeClusters {
		if !strings.HasPrefix(rn, "outpost-") {
			continue
		}
		display := strings.TrimPrefix(rn, "outpost-")
		if seen[display] {
			continue
		}
		nodes, _ := nodeCountFor(ctx, s.Exec, drv, rn)
		result = append(result, Cluster{
			Name: display, Driver: drv.String(), KindName: rn, Status: "ready", NodeCount: nodes,
		})
	}
	return result, nil
}

func (s *Service) Status(ctx context.Context, name string) (*Cluster, error) {
	clusters, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	safe := config.SanitizeClusterName(name)
	for _, c := range clusters {
		if c.Name == safe {
			return &c, nil
		}
	}
	return nil, fmt.Errorf("cluster %q not found", name)
}

func (s *Service) Delete(ctx context.Context, name string) error {
	runtimeName, driver := resolveClusterTarget(ctx, s.Exec, name)
	if err := deleteCluster(ctx, s.Exec, driver, runtimeName); err != nil {
		return err
	}
	remoteDir := RemoteDir(name)
	_, _ = s.Exec.Run(ctx, fmt.Sprintf("rm -rf %s", shellQuote(remoteDir)), transport.RunOpts{})
	if s.HostName != "" {
		if local, lerr := LocalKubeconfigPath(s.HostName, name); lerr == nil {
			_ = os.Remove(local)
		}
	}
	return nil
}

func (s *Service) listMeta(ctx context.Context) ([]Meta, error) {
	out, err := inspect.RunOutput(ctx, s.Exec, fmt.Sprintf("ls -1 %s 2>/dev/null || true", shellQuote(remoteBase)))
	if err != nil {
		return nil, err
	}
	var metas []Meta
	for _, dir := range strings.Split(strings.TrimSpace(out), "\n") {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		data, err := s.Exec.Download(remoteBase + "/" + dir + "/meta.yaml")
		if err != nil {
			continue
		}
		var m Meta
		if yaml.Unmarshal(data, &m) == nil {
			metas = append(metas, m)
		}
	}
	return metas, nil
}

func (s *Service) syncLocalKubeconfig(name, content string) error {
	if s.HostName == "" {
		return nil
	}
	path, err := LocalKubeconfigPath(s.HostName, name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0600)
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func Count(ctx context.Context, exec transport.Executor) (int, error) {
	svc := &Service{Exec: exec}
	list, err := svc.List(ctx)
	if err != nil {
		return 0, err
	}
	return len(list), nil
}
