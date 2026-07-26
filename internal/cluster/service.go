package cluster

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goke/outpost/internal/bootstrap"
	"github.com/goke/outpost/internal/capacity"
	"github.com/goke/outpost/internal/config"
	"github.com/goke/outpost/internal/inspect"
	"github.com/goke/outpost/internal/output"
	"github.com/goke/outpost/internal/transport"
	"gopkg.in/yaml.v3"
)

type Cluster struct {
	Name          string `json:"name"`
	KindName      string `json:"kind_name"`
	Workers       int    `json:"workers"`
	ControlPlanes int    `json:"control_planes"`
	Status        string `json:"status"`
	NodeCount     int    `json:"node_count"`
}

type Meta struct {
	Name          string    `yaml:"name"`
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

func (s *Service) Create(ctx context.Context, name string, workers, controlPlanes int) error {
	safe := config.SanitizeClusterName(name)
	if safe == "" {
		return fmt.Errorf("cluster name is required")
	}
	kindName := KindName(name)
	if controlPlanes == 0 {
		controlPlanes = 1
	}
	if err := bootstrap.Ensure(ctx, s.Exec); err != nil {
		return err
	}
	if err := bootstrap.EnsureKubernetesTools(ctx, s.Exec); err != nil {
		return err
	}
	cpu, mem, disk := EstimateResources(controlPlanes, workers)
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
	cfg := RenderKindConfig(KindConfig{Name: kindName, ControlPlanes: controlPlanes, Workers: workers})
	cfgPath := remoteDir + "/kind-config.yaml"
	if err := s.Exec.UploadBytes([]byte(cfg), cfgPath); err != nil {
		return err
	}

	createCmd := fmt.Sprintf("kind create cluster --name %s --config %s", shellQuote(kindName), shellQuote(cfgPath))
	code, err := s.Exec.Run(ctx, createCmd, transport.RunOpts{})
	if err != nil {
		_ = s.deleteKindCluster(ctx, kindName)
		return err
	}
	if code != 0 {
		_ = s.deleteKindCluster(ctx, kindName)
		return fmt.Errorf("kind create cluster failed (exit %d)", code)
	}

	kubeCmd := fmt.Sprintf("kind get kubeconfig --name %s", shellQuote(kindName))
	kubeOut, err := inspect.RunOutput(ctx, s.Exec, kubeCmd)
	if err != nil {
		_ = s.deleteKindCluster(ctx, kindName)
		return fmt.Errorf("fetch kubeconfig: %w", err)
	}
	kubePath := RemoteKubeconfig(name)
	if err := s.Exec.UploadBytes([]byte(kubeOut), kubePath); err != nil {
		_ = s.deleteKindCluster(ctx, kindName)
		return err
	}
	_, _ = s.Exec.Run(ctx, fmt.Sprintf("chmod 600 %s", shellQuote(kubePath)), transport.RunOpts{})

	meta := Meta{
		Name: safe, KindName: kindName, Workers: workers, ControlPlanes: controlPlanes, CreatedAt: time.Now().UTC(),
	}
	metaBytes, _ := yaml.Marshal(meta)
	if err := s.Exec.UploadBytes(metaBytes, remoteDir+"/meta.yaml"); err != nil {
		_ = s.deleteKindCluster(ctx, kindName)
		return err
	}
	if err := s.syncLocalKubeconfig(name, kubeOut); err != nil {
		return err
	}
	if s.Out != nil && !s.Out.JSON {
		s.Out.Success("Cluster %q is ready (%d control-plane, %d workers)", name, controlPlanes, workers)
	}
	return nil
}

func (s *Service) List(ctx context.Context) ([]Cluster, error) {
	out, err := inspect.RunOutput(ctx, s.Exec, "kind get clusters 2>/dev/null || true")
	if err != nil {
		return nil, err
	}
	kindNames := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			kindNames[line] = true
		}
	}
	metaClusters, _ := s.listMeta(ctx)
	var result []Cluster
	seen := map[string]bool{}
	for _, m := range metaClusters {
		status := "unknown"
		if kindNames[m.KindName] {
			status = "ready"
		}
		nodes, _ := s.nodeCount(ctx, m.KindName)
		result = append(result, Cluster{
			Name: m.Name, KindName: m.KindName, Workers: m.Workers,
			ControlPlanes: m.ControlPlanes, Status: status, NodeCount: nodes,
		})
		seen[m.Name] = true
	}
	for kn := range kindNames {
		if !strings.HasPrefix(kn, "outpost-") {
			continue
		}
		display := strings.TrimPrefix(kn, "outpost-")
		if seen[display] {
			continue
		}
		nodes, _ := s.nodeCount(ctx, kn)
		result = append(result, Cluster{
			Name: display, KindName: kn, Status: "ready", NodeCount: nodes,
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
	safe := config.SanitizeClusterName(name)
	meta, err := s.loadMeta(ctx, safe)
	kindName := KindName(name)
	if err == nil {
		kindName = meta.KindName
	}
	if err := s.deleteKindCluster(ctx, kindName); err != nil {
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

func (s *Service) deleteKindCluster(ctx context.Context, kindName string) error {
	cmd := fmt.Sprintf("kind delete cluster --name %s 2>/dev/null || true", shellQuote(kindName))
	_, err := s.Exec.Run(ctx, cmd, transport.RunOpts{})
	return err
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

func (s *Service) loadMeta(ctx context.Context, safeName string) (*Meta, error) {
	data, err := s.Exec.Download(remoteBase + "/" + safeName + "/meta.yaml")
	if err != nil {
		return nil, err
	}
	var m Meta
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Service) nodeCount(ctx context.Context, kindName string) (int, error) {
	cmd := fmt.Sprintf("docker ps --filter label=io.x-k8s.kind.cluster=%s --format '{{.ID}}' | wc -l", shellQuote(kindName))
	out, err := inspect.RunOutput(ctx, s.Exec, cmd)
	if err != nil {
		return 0, err
	}
	var n int
	fmt.Sscanf(strings.TrimSpace(out), "%d", &n)
	return n, nil
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
