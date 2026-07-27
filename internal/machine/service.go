package machine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/goke/outpost/internal/bootstrap"
	"github.com/goke/outpost/internal/capabilities"
	"github.com/goke/outpost/internal/capacity"
	"github.com/goke/outpost/internal/config"
	"github.com/goke/outpost/internal/inspect"
	"github.com/goke/outpost/internal/output"
	"github.com/goke/outpost/internal/transport"
	"gopkg.in/yaml.v3"
)

type Machine struct {
	Name        string  `json:"name"`
	IncusName   string  `json:"incus_name"`
	Type        string  `json:"type"`
	Image       string  `json:"image"`
	Status      string  `json:"status"`
	CPU         float64 `json:"cpu"`
	MemoryBytes uint64  `json:"memory_bytes"`
	DiskBytes   uint64  `json:"disk_bytes"`
	IPv4        string  `json:"ipv4,omitempty"`
}

type Meta struct {
	Name        string    `yaml:"name"`
	IncusName   string    `yaml:"incus_name"`
	Type        string    `yaml:"type"`
	Image       string    `yaml:"image"`
	CPU         float64   `yaml:"cpu"`
	MemoryBytes uint64    `yaml:"memory_bytes"`
	DiskBytes   uint64    `yaml:"disk_bytes"`
	CreatedAt   time.Time `yaml:"created_at"`
}

type DeleteInfo struct {
	DiskBytes     uint64 `json:"disk_bytes"`
	SnapshotCount int    `json:"snapshot_count"`
	SnapshotBytes uint64 `json:"snapshot_bytes,omitempty"`
}

type Service struct {
	Exec     transport.Executor
	Out      *output.Printer
	HostName string
}

type incusListEntry struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Type   string `json:"type"`
	State  struct {
		Network map[string]struct {
			Addresses []struct {
				Family  string `json:"family"`
				Address string `json:"address"`
			} `json:"addresses"`
		} `json:"network"`
	} `json:"state"`
}

func (s *Service) Create(ctx context.Context, name string, opts CreateOptions, providerMeta *config.ProviderMeta) error {
	safe := config.SanitizeMachineName(name)
	if safe == "" {
		return fmt.Errorf("machine name is required")
	}
	if strings.TrimSpace(opts.Image) == "" {
		return fmt.Errorf("--image is required")
	}
	opts.ApplyDefaults()

	if opts.VirtualMachine {
		vmSupport, err := capabilities.VMSupport(ctx, s.Exec, providerMeta)
		if err != nil {
			return err
		}
		if !vmSupport.CanCreateVM() {
			return fmt.Errorf("%s\n\n%s", vmSupport.Reason, vmSupport.Remediation)
		}
	}

	cpu, mem, disk := EstimateResources(opts)
	if err := capacity.Check(ctx, s.Exec, capacity.Request{CPUCores: cpu, MemoryBytes: mem, DiskBytes: disk}); err != nil {
		return fmt.Errorf("%w; use --memory, --cpu, or --disk to request less, or run 'outpost capacity' to inspect the host", err)
	}

	if err := bootstrap.Ensure(ctx, s.Exec); err != nil {
		return err
	}
	if err := bootstrap.EnsureIncus(ctx, s.Exec); err != nil {
		return err
	}

	existing, _ := s.List(ctx)
	for _, m := range existing {
		if m.Name == safe {
			return fmt.Errorf("machine %q already exists", name)
		}
	}

	incusName := IncusName(name)
	remoteDir := RemoteDir(name)
	if err := transport.EnsureRemoteDir(s.Exec, remoteDir); err != nil {
		return err
	}

	if err := s.validateImage(ctx, opts.Image); err != nil {
		return err
	}

	memLimit := FormatSize(opts.MemoryBytes)
	diskLimit := FormatSize(opts.DiskBytes)
	launchParts := []string{
		"incus launch",
		shellQuote(opts.Image),
		shellQuote(incusName),
		fmt.Sprintf("-c limits.cpu=%g", opts.CPU),
		fmt.Sprintf("-c limits.memory=%s", memLimit),
		fmt.Sprintf("-d root,size=%s", diskLimit),
	}
	if opts.VirtualMachine {
		launchParts = append(launchParts, "--vm")
	}
	createCmd := strings.Join(launchParts, " ")

	code, err := s.Exec.Run(ctx, createCmd, transport.RunOpts{})
	if err != nil {
		_ = s.deleteIncusInstance(ctx, incusName)
		return err
	}
	if code != 0 {
		_ = s.deleteIncusInstance(ctx, incusName)
		return fmt.Errorf("incus launch failed (exit %d)", code)
	}

	meta := Meta{
		Name:        safe,
		IncusName:   incusName,
		Type:        opts.MachineType(),
		Image:       opts.Image,
		CPU:         opts.CPU,
		MemoryBytes: opts.MemoryBytes,
		DiskBytes:   opts.DiskBytes,
		CreatedAt:   time.Now().UTC(),
	}
	metaBytes, _ := yaml.Marshal(meta)
	if err := s.Exec.UploadBytes(metaBytes, remoteDir+"/meta.yaml"); err != nil {
		_ = s.deleteIncusInstance(ctx, incusName)
		return err
	}

	if s.Out != nil && !s.Out.JSON {
		s.Out.Success("Machine %q is ready (%s)", name, TypeLabel(meta.Type))
	}
	return nil
}

func (s *Service) List(ctx context.Context) ([]Machine, error) {
	runtime, _ := s.listRuntime(ctx)
	metaMachines, _ := s.listMeta(ctx)
	var result []Machine
	seen := map[string]bool{}
	for _, m := range metaMachines {
		status := "unknown"
		ipv4 := ""
		if rt, ok := runtime[m.IncusName]; ok {
			status = strings.ToLower(rt.Status)
			ipv4 = rt.IPv4
		}
		result = append(result, Machine{
			Name:        m.Name,
			IncusName:   m.IncusName,
			Type:        m.Type,
			Image:       m.Image,
			Status:      status,
			CPU:         m.CPU,
			MemoryBytes: m.MemoryBytes,
			DiskBytes:   m.DiskBytes,
			IPv4:        ipv4,
		})
		seen[m.Name] = true
	}
	for incusName, rt := range runtime {
		if !strings.HasPrefix(incusName, "outpost-") {
			continue
		}
		display := strings.TrimPrefix(incusName, "outpost-")
		if seen[display] {
			continue
		}
		machineType := TypeContainer
		if strings.EqualFold(rt.Type, "virtual-machine") {
			machineType = TypeVM
		}
		result = append(result, Machine{
			Name:      display,
			IncusName: incusName,
			Type:      machineType,
			Status:    strings.ToLower(rt.Status),
			IPv4:      rt.IPv4,
		})
	}
	return result, nil
}

func (s *Service) Status(ctx context.Context, name string) (*Machine, error) {
	machines, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	safe := config.SanitizeMachineName(name)
	for _, m := range machines {
		if m.Name == safe {
			return &m, nil
		}
	}
	return nil, fmt.Errorf("machine %q not found", name)
}

func (s *Service) Start(ctx context.Context, name string) error {
	incusName, err := s.resolveIncusName(ctx, name)
	if err != nil {
		return err
	}
	return s.runIncusAction(ctx, "start", incusName)
}

func (s *Service) Stop(ctx context.Context, name string) error {
	incusName, err := s.resolveIncusName(ctx, name)
	if err != nil {
		return err
	}
	return s.runIncusAction(ctx, "stop", incusName)
}

func (s *Service) Restart(ctx context.Context, name string) error {
	incusName, err := s.resolveIncusName(ctx, name)
	if err != nil {
		return err
	}
	return s.runIncusAction(ctx, "restart", incusName)
}

func (s *Service) SnapshotCreate(ctx context.Context, name, snapName string) error {
	incusName, err := s.resolveIncusName(ctx, name)
	if err != nil {
		return err
	}
	if strings.TrimSpace(snapName) == "" {
		snapName = fmt.Sprintf("snap-%d", time.Now().Unix())
	}
	cmd := fmt.Sprintf("incus snapshot create %s %s", shellQuote(incusName), shellQuote(snapName))
	code, err := s.Exec.Run(ctx, cmd, transport.RunOpts{})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("incus snapshot create failed (exit %d)", code)
	}
	return nil
}

func (s *Service) SnapshotList(ctx context.Context, name string) ([]string, error) {
	incusName, err := s.resolveIncusName(ctx, name)
	if err != nil {
		return nil, err
	}
	cmd := fmt.Sprintf("incus list %s/snapshots -c n --format csv", shellQuote(incusName))
	out, err := inspect.RunOutput(ctx, s.Exec, cmd)
	if err != nil {
		return nil, err
	}
	var snaps []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			snaps = append(snaps, line)
		}
	}
	return snaps, nil
}

func (s *Service) SnapshotDelete(ctx context.Context, name, snapName string) error {
	incusName, err := s.resolveIncusName(ctx, name)
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf("incus delete %s/%s", shellQuote(incusName), shellQuote(snapName))
	code, err := s.Exec.Run(ctx, cmd, transport.RunOpts{})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("incus snapshot delete failed (exit %d)", code)
	}
	return nil
}

func (s *Service) DeleteInfo(ctx context.Context, name string) (*DeleteInfo, error) {
	meta, err := s.loadMeta(ctx, config.SanitizeMachineName(name))
	if err != nil {
		return nil, err
	}
	info := &DeleteInfo{DiskBytes: meta.DiskBytes}
	if disk, err := s.instanceDiskBytes(ctx, meta.IncusName); err == nil && disk > 0 {
		info.DiskBytes = disk
	}
	snaps, err := s.SnapshotList(ctx, name)
	if err == nil {
		info.SnapshotCount = len(snaps)
		if info.SnapshotCount > 0 {
			info.SnapshotBytes = info.DiskBytes * uint64(info.SnapshotCount)
		}
	}
	return info, nil
}

func (s *Service) validateImage(ctx context.Context, image string) error {
	cmd := fmt.Sprintf("incus image info %s >/dev/null 2>&1", shellQuote(image))
	code, err := s.Exec.Run(ctx, cmd, transport.RunOpts{})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("image %q not found on host — run 'incus image list' on the host or choose a different alias", image)
	}
	return nil
}

func (s *Service) instanceDiskBytes(ctx context.Context, incusName string) (uint64, error) {
	cmd := fmt.Sprintf("incus config device get %s root size 2>/dev/null || true", shellQuote(incusName))
	out, err := inspect.RunOutput(ctx, s.Exec, cmd)
	if err != nil {
		return 0, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return 0, fmt.Errorf("no disk size")
	}
	return ParseSize(out)
}

func (s *Service) Delete(ctx context.Context, name string) error {
	safe := config.SanitizeMachineName(name)
	meta, err := s.loadMeta(ctx, safe)
	incusName := IncusName(name)
	if err == nil {
		incusName = meta.IncusName
	}
	if err := s.deleteIncusInstance(ctx, incusName); err != nil {
		return err
	}
	remoteDir := RemoteDir(name)
	_, _ = s.Exec.Run(ctx, fmt.Sprintf("rm -rf %s", shellQuote(remoteDir)), transport.RunOpts{})
	return nil
}

func (s *Service) runIncusAction(ctx context.Context, action, incusName string) error {
	cmd := fmt.Sprintf("incus %s %s", action, shellQuote(incusName))
	code, err := s.Exec.Run(ctx, cmd, transport.RunOpts{})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("incus %s failed (exit %d)", action, code)
	}
	return nil
}

func (s *Service) deleteIncusInstance(ctx context.Context, incusName string) error {
	cmd := fmt.Sprintf("incus delete %s --force 2>/dev/null || true", shellQuote(incusName))
	_, err := s.Exec.Run(ctx, cmd, transport.RunOpts{})
	return err
}

type runtimeInfo struct {
	Status string
	Type   string
	IPv4   string
}

func (s *Service) listRuntime(ctx context.Context) (map[string]runtimeInfo, error) {
	out, err := inspect.RunOutput(ctx, s.Exec, "incus list --format json 2>/dev/null || true")
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)
	if out == "" || out == "[]" {
		return map[string]runtimeInfo{}, nil
	}
	var entries []incusListEntry
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		return nil, err
	}
	result := make(map[string]runtimeInfo, len(entries))
	for _, e := range entries {
		ipv4 := ""
		for _, net := range e.State.Network {
			for _, addr := range net.Addresses {
				if addr.Family == "inet" && addr.Address != "" && !strings.HasPrefix(addr.Address, "127.") {
					ipv4 = addr.Address
					break
				}
			}
			if ipv4 != "" {
				break
			}
		}
		result[e.Name] = runtimeInfo{
			Status: e.Status,
			Type:   e.Type,
			IPv4:   ipv4,
		}
	}
	return result, nil
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

func (s *Service) resolveIncusName(ctx context.Context, name string) (string, error) {
	safe := config.SanitizeMachineName(name)
	meta, err := s.loadMeta(ctx, safe)
	if err == nil {
		return meta.IncusName, nil
	}
	runtime, _ := s.listRuntime(ctx)
	incusName := IncusName(name)
	if _, ok := runtime[incusName]; ok {
		return incusName, nil
	}
	return "", fmt.Errorf("machine %q not found", name)
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

func ListStoppedOutpost(ctx context.Context, exec transport.Executor) ([]string, error) {
	svc := &Service{Exec: exec}
	runtime, err := svc.listRuntime(ctx)
	if err != nil {
		return nil, err
	}
	var stopped []string
	for name, info := range runtime {
		if !strings.HasPrefix(name, "outpost-") {
			continue
		}
		if strings.EqualFold(info.Status, "stopped") {
			stopped = append(stopped, name)
		}
	}
	return stopped, nil
}
