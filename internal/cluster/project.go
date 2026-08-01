package cluster

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/degoke/outpost/internal/bootstrap"
	"github.com/degoke/outpost/internal/capacity"
	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/environment"
	"github.com/degoke/outpost/internal/inspect"
	"github.com/degoke/outpost/internal/output"
	"github.com/degoke/outpost/internal/transport"
	"gopkg.in/yaml.v3"
)

const (
	defaultProjectDriver = DriverKind
	projectStateName     = "kubernetes"
)

// ProjectService manages the single Kubernetes cluster belonging to a project.
// Kubernetes commands run inside the managed project container; state files
// and file transfers remain on the remote host alongside the project volume.
type ProjectService struct {
	HostExec transport.Executor
	Project  *config.Project
	Cwd      string
	Out      *output.Printer

	manager   *environment.Manager
	container *environment.ContainerExecutor
}

func NewProjectService(hostExec transport.Executor, project *config.Project, cwd string, out *output.Printer) *ProjectService {
	m := environment.New(hostExec, project, cwd)
	return &ProjectService{
		HostExec:  hostExec,
		Project:   project,
		Cwd:       cwd,
		Out:       out,
		manager:   m,
		container: &environment.ContainerExecutor{Host: hostExec, Manager: m},
	}
}

func (s *ProjectService) ensure(ctx context.Context) error {
	if s.Project == nil {
		return fmt.Errorf("project is required")
	}
	if !s.Project.EnvironmentEnabled() {
		return fmt.Errorf("project Kubernetes requires the managed project container — remove environment.enabled: false from project.yaml")
	}
	if err := s.manager.Ensure(ctx); err != nil {
		return err
	}
	if err := bootstrap.EnsureKubernetesTools(ctx, s.container); err != nil {
		return fmt.Errorf("prepare Kubernetes tools in project container: %w", err)
	}
	code, err := s.container.Run(ctx, "docker info >/dev/null 2>&1", transport.RunOpts{})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("project container cannot access Docker — enable environment.docker_socket and ensure the container user can use /var/run/docker.sock")
	}
	return nil
}

func (s *ProjectService) stateHostDir() string {
	return filepath.Join(s.Project.RemoteDir, ".outpost", projectStateName)
}

func (s *ProjectService) stateContainerDir() string {
	return filepath.Join(s.manager.Workdir(), ".outpost", projectStateName)
}

func (s *ProjectService) stateHostPath(name string) string {
	return filepath.Join(s.stateHostDir(), name)
}

func (s *ProjectService) stateContainerPath(name string) string {
	return filepath.Join(s.stateContainerDir(), name)
}

func (s *ProjectService) runtimeName() string {
	return KindName(s.Project.Name)
}

func (s *ProjectService) configuredDriver() (Driver, error) {
	if s.Project.Kubernetes == nil || strings.TrimSpace(s.Project.Kubernetes.Driver) == "" {
		return defaultProjectDriver, nil
	}
	return ParseDriver(s.Project.Kubernetes.Driver)
}

// Up creates the project cluster if necessary. driverFlag is empty when the
// saved project driver (or the kind default) should be used.
func (s *ProjectService) Up(ctx context.Context, driverFlag string) error {
	var driver Driver
	var err error
	if strings.TrimSpace(driverFlag) != "" {
		driver, err = ParseDriver(driverFlag)
		if err != nil {
			return err
		}
	} else {
		driver, err = s.configuredDriver()
		if err != nil {
			return err
		}
	}
	// Mark Kubernetes as enabled before ensuring the container so a newly
	// created project container receives the host gateway needed by kind/k3d.
	// Persisting the setting is deferred until any existing-driver conflict has
	// been checked below.
	configChanged := false
	if s.Project.Kubernetes == nil {
		s.Project.Kubernetes = &config.ProjectKubernetes{Driver: driver.String()}
		configChanged = true
	} else if strings.TrimSpace(s.Project.Kubernetes.Driver) == "" {
		s.Project.Kubernetes.Driver = driver.String()
		configChanged = true
	}

	if err := s.ensure(ctx); err != nil {
		return err
	}
	existingDriver, exists, err := s.detectExisting(ctx)
	if err != nil {
		return err
	}
	if exists && existingDriver != driver {
		return fmt.Errorf("project cluster already uses %s; run 'outpost cluster down' before switching to %s", existingDriver, driver)
	}

	if strings.TrimSpace(driverFlag) != "" || configChanged {
		s.Project.Kubernetes.Driver = driver.String()
		if err := config.SaveProject(s.Cwd, s.Project); err != nil {
			return err
		}
	}

	if err := transport.EnsureRemoteDir(s.HostExec, s.stateHostDir()); err != nil {
		return err
	}
	if exists {
		if err := s.persistKubeconfig(ctx, driver); err != nil {
			return err
		}
		if s.Out != nil && !s.Out.JSON {
			s.Out.Success("Project Kubernetes cluster is ready (%s)", driver)
		}
		return nil
	}

	cpu, mem, disk := EstimateResources(driver, 1, 0)
	if err := capacity.Check(ctx, s.HostExec, capacity.Request{CPUCores: cpu, MemoryBytes: mem, DiskBytes: disk}); err != nil {
		return err
	}

	configContainerPath := s.stateContainerPath("kind-config.yaml")
	if driver == DriverKind {
		cfg := RenderKindConfig(KindConfig{Name: s.runtimeName(), ControlPlanes: 1})
		if err := s.HostExec.UploadBytes([]byte(cfg), s.stateHostPath("kind-config.yaml")); err != nil {
			return err
		}
	}

	if s.Out != nil {
		s.Out.Step("Creating project Kubernetes cluster with %s...", driver)
	}
	code, runErr := createCluster(ctx, s.container, driver, s.runtimeName(), 0, 1, configContainerPath)
	if runErr != nil {
		return runErr
	}
	if code != 0 {
		return fmt.Errorf("%s cluster create failed (exit %d)", driver, code)
	}
	if err := s.persistKubeconfig(ctx, driver); err != nil {
		_ = deleteCluster(ctx, s.container, driver, s.runtimeName())
		return err
	}
	meta := Meta{
		Name: config.SanitizeProjectName(s.Project.Name), Driver: driver.String(), KindName: s.runtimeName(),
		Workers: 0, ControlPlanes: 1, CreatedAt: time.Now().UTC(),
	}
	metaBytes, _ := yaml.Marshal(meta)
	if err := s.HostExec.UploadBytes(metaBytes, s.stateHostPath("meta.yaml")); err != nil {
		_ = deleteCluster(ctx, s.container, driver, s.runtimeName())
		return err
	}
	if s.Out != nil && !s.Out.JSON {
		s.Out.Success("Project Kubernetes cluster is ready (%s)", driver)
	}
	return nil
}

func (s *ProjectService) persistKubeconfig(ctx context.Context, driver Driver) error {
	if s.Out != nil {
		s.Out.Step("Saving project kubeconfig...")
	}
	kubeOut, err := fetchKubeconfig(ctx, s.container, driver, s.runtimeName())
	if err != nil {
		return fmt.Errorf("fetch kubeconfig: %w", err)
	}
	port, err := s.APIPort([]byte(kubeOut))
	if err != nil {
		return err
	}
	// kind's default API address is host loopback. Since the CLI runs inside
	// the project container, persist a container-reachable host address; the
	// local copy generated by `open` is rewritten again to 127.0.0.1.
	kubeBytes, err := RewriteKubeconfigServerURL([]byte(kubeOut), fmt.Sprintf("https://host.docker.internal:%d", port))
	if err != nil {
		return err
	}
	if err := s.HostExec.UploadBytes(kubeBytes, s.stateHostPath("kubeconfig")); err != nil {
		return err
	}
	code, err := s.HostExec.Run(ctx, fmt.Sprintf("chmod 600 %s", shellQuote(s.stateHostPath("kubeconfig"))), transport.RunOpts{})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("could not protect project kubeconfig")
	}
	return nil
}

func (s *ProjectService) detectExisting(ctx context.Context) (Driver, bool, error) {
	var found Driver
	for _, driver := range []Driver{DriverKind, DriverK3d} {
		out, err := inspect.RunOutput(ctx, s.container, runtimeListCommand(driver))
		if err != nil {
			return "", false, err
		}
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			if strings.TrimSpace(line) == s.runtimeName() {
				if found != "" && found != driver {
					return "", false, fmt.Errorf("project has both kind and k3d clusters named %q; remove one manually before continuing", s.runtimeName())
				}
				found = driver
			}
		}
	}
	return found, found != "", nil
}

func runtimeListCommand(driver Driver) string {
	if driver == DriverK3d {
		return "k3d cluster list --no-headers 2>/dev/null | awk '{print $1}' || true"
	}
	return "kind get clusters 2>/dev/null || true"
}

// Down deletes the project cluster but leaves the project container intact.
func (s *ProjectService) Down(ctx context.Context) error {
	if err := s.ensure(ctx); err != nil {
		return err
	}
	driver, exists, err := s.detectExisting(ctx)
	if err != nil {
		return err
	}
	if exists {
		if err := deleteClusterStrict(ctx, s.container, driver, s.runtimeName()); err != nil {
			return err
		}
	}
	code, err := s.HostExec.Run(ctx, fmt.Sprintf("rm -rf %s", shellQuote(s.stateHostDir())), transport.RunOpts{})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("could not remove project Kubernetes state")
	}
	_ = os.Remove(LocalProjectKubeconfigPath(s.Cwd))
	return nil
}

func (s *ProjectService) Status(ctx context.Context) (*Cluster, error) {
	if err := s.ensure(ctx); err != nil {
		return nil, err
	}
	driver, exists, err := s.detectExisting(ctx)
	if err != nil {
		return nil, err
	}
	if !exists {
		driver, err = s.configuredDriver()
		if err != nil {
			return nil, err
		}
	}
	status := "missing"
	nodes := 0
	if exists {
		status = "ready"
		nodes, _ = nodeCountFor(ctx, s.container, driver, s.runtimeName())
	}
	return &Cluster{
		Name: config.SanitizeProjectName(s.Project.Name), Driver: driver.String(), KindName: s.runtimeName(),
		Workers: 0, ControlPlanes: 1, Status: status, NodeCount: nodes,
	}, nil
}

func (s *ProjectService) Kubeconfig() ([]byte, error) {
	return s.HostExec.Download(s.stateHostPath("kubeconfig"))
}

// APIPort extracts the host-local API port from the project kubeconfig. kind
// and k3d publish the API endpoint on the remote Docker host, which the SSH
// forward can reach through 127.0.0.1.
func (s *ProjectService) APIPort(kubeconfig []byte) (int, error) {
	var doc struct {
		Clusters []struct {
			Cluster struct {
				Server string `yaml:"server"`
			} `yaml:"cluster"`
		} `yaml:"clusters"`
	}
	if err := yaml.Unmarshal(kubeconfig, &doc); err != nil {
		return 0, fmt.Errorf("parse kubeconfig: %w", err)
	}
	if len(doc.Clusters) == 0 || strings.TrimSpace(doc.Clusters[0].Cluster.Server) == "" {
		return 0, fmt.Errorf("kubeconfig has no Kubernetes API server")
	}
	u, err := url.Parse(doc.Clusters[0].Cluster.Server)
	if err != nil || u.Port() == "" {
		return 0, fmt.Errorf("kubeconfig API server has no TCP port")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid Kubernetes API port %q", u.Port())
	}
	return port, nil
}

func LocalProjectKubeconfigPath(cwd string) string {
	return filepath.Join(cwd, ".outpost", "kubeconfig")
}

func RewriteKubeconfigServer(data []byte, localPort int) ([]byte, error) {
	return RewriteKubeconfigServerURL(data, fmt.Sprintf("https://127.0.0.1:%d", localPort))
}

func RewriteKubeconfigServerURL(data []byte, serverURL string) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %w", err)
	}
	if len(doc.Content) == 0 {
		return nil, fmt.Errorf("kubeconfig is empty")
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("kubeconfig root is not a mapping")
	}
	clusters := mappingValue(root, "clusters")
	if clusters == nil || clusters.Kind != yaml.SequenceNode || len(clusters.Content) == 0 {
		return nil, fmt.Errorf("kubeconfig has no clusters")
	}
	clusterMap := clusters.Content[0]
	cluster := mappingValue(clusterMap, "cluster")
	if cluster == nil {
		return nil, fmt.Errorf("kubeconfig has no cluster data")
	}
	server := mappingValue(cluster, "server")
	if server == nil {
		return nil, fmt.Errorf("kubeconfig has no API server")
	}
	server.Value = serverURL
	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	if err := enc.Encode(&doc); err != nil {
		return nil, err
	}
	_ = enc.Close()
	return out.Bytes(), nil
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}
