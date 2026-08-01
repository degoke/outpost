package machine

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/output"
	"github.com/degoke/outpost/internal/transport"
	"gopkg.in/yaml.v3"
)

// ProjectService exposes the single Incus machine owned by a project.
// Incus still runs on the remote host; the project owns its name and metadata.
type ProjectService struct {
	project *config.Project
	service *Service
}

func NewProjectService(exec transport.Executor, project *config.Project, out *output.Printer) *ProjectService {
	return &ProjectService{
		project: project,
		service: &Service{
			Exec:          exec,
			Out:           out,
			RemoteDirBase: strings.TrimRight(project.RemoteDir, "/") + "/.outpost/machines",
		},
	}
}

func (s *ProjectService) name() string { return s.project.Name }

func (s *ProjectService) Up(ctx context.Context, opts CreateOptions, provider *config.ProviderMeta) error {
	if existing, err := s.Status(ctx); err == nil {
		if s.service.Out != nil && !s.service.Out.JSON {
			s.service.Out.Info("Project machine already exists (%s)", existing.Status)
		}
		return nil
	}
	if err := s.service.Create(ctx, s.name(), opts, provider); err != nil {
		return err
	}
	return nil
}

func (s *ProjectService) Status(ctx context.Context) (*Machine, error) {
	meta, err := s.service.loadMeta(ctx, config.SanitizeMachineName(s.name()))
	if err != nil {
		return nil, fmt.Errorf("project machine is not created — run 'outpost machine up'")
	}
	runtime, err := s.service.listRuntime(ctx)
	if err != nil {
		return nil, err
	}
	status, ipv4 := "unknown", ""
	if rt, ok := runtime[meta.IncusName]; ok {
		status, ipv4 = strings.ToLower(rt.Status), rt.IPv4
	}
	return &Machine{Name: meta.Name, IncusName: meta.IncusName, Type: meta.Type, Image: meta.Image, Status: status, CPU: meta.CPU, MemoryBytes: meta.MemoryBytes, DiskBytes: meta.DiskBytes, IPv4: ipv4}, nil
}

func (s *ProjectService) Down(ctx context.Context) error {
	return s.service.Delete(ctx, s.name())
}

func (s *ProjectService) Shell(ctx context.Context) error {
	return s.service.Shell(ctx, s.name())
}

func (s *ProjectService) RunCommand(ctx context.Context, args []string) (int, error) {
	return s.service.RunCommand(ctx, s.name(), args)
}

func (s *ProjectService) Copy(ctx context.Context, src, dst string, recursive bool) error {
	// Project machine paths intentionally omit a machine name: project://path
	src = normalizeProjectPath(src, s.name())
	dst = normalizeProjectPath(dst, s.name())
	return s.service.Copy(ctx, src, dst, recursive)
}

func (s *ProjectService) StartConnect(ctx context.Context, portSpecs []string, bindHost string) ([]ConnectForward, []io.Closer, error) {
	return s.service.StartConnect(ctx, s.name(), portSpecs, bindHost)
}

func (s *ProjectService) Start(ctx context.Context) error   { return s.service.Start(ctx, s.name()) }
func (s *ProjectService) Stop(ctx context.Context) error    { return s.service.Stop(ctx, s.name()) }
func (s *ProjectService) Restart(ctx context.Context) error { return s.service.Restart(ctx, s.name()) }

func (s *ProjectService) SnapshotCreate(ctx context.Context, name string) error {
	return s.service.SnapshotCreate(ctx, s.name(), name)
}
func (s *ProjectService) SnapshotList(ctx context.Context) ([]string, error) {
	return s.service.SnapshotList(ctx, s.name())
}
func (s *ProjectService) SnapshotDelete(ctx context.Context, name string) error {
	return s.service.SnapshotDelete(ctx, s.name(), name)
}

// Exists reports whether the project machine is present on the remote host.
func (s *ProjectService) Exists(ctx context.Context) bool {
	_, err := s.Status(ctx)
	return err == nil
}

// ExportArchive stops the project machine and writes an Incus export tarball to remoteArchive.
func (s *ProjectService) ExportArchive(ctx context.Context, remoteArchive string) error {
	if !s.Exists(ctx) {
		return fmt.Errorf("project machine is not created")
	}
	incusName, err := s.service.resolveIncusName(ctx, s.name())
	if err != nil {
		return err
	}
	stopCmd, err := s.service.incusCommand(ctx, fmt.Sprintf("stop %s --force", shellQuote(incusName)))
	if err != nil {
		return err
	}
	if _, err := s.service.Exec.Run(ctx, stopCmd, transport.RunOpts{}); err != nil {
		return err
	}
	exportCmd, err := s.service.incusCommand(ctx, fmt.Sprintf("export %s %s", shellQuote(incusName), shellQuote(remoteArchive)))
	if err != nil {
		return err
	}
	code, err := s.service.Exec.Run(ctx, exportCmd, transport.RunOpts{})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("incus export failed (exit %d)", code)
	}
	return nil
}

// ImportArchive imports a previously exported Incus tarball from remoteArchive.
func (s *ProjectService) ImportArchive(ctx context.Context, remoteArchive string, provider *config.ProviderMeta) error {
	if s.Exists(ctx) {
		return nil
	}
	importCmd, err := s.service.incusCommand(ctx, fmt.Sprintf("import %s", shellQuote(remoteArchive)))
	if err != nil {
		return err
	}
	code, err := s.service.Exec.Run(ctx, importCmd, transport.RunOpts{})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("incus import failed (exit %d)", code)
	}
	if _, err := s.Status(ctx); err != nil {
		incusName := s.service.machineIncusName(s.name())
		if rt, listErr := s.service.listRuntime(ctx); listErr == nil {
			for name := range rt {
				if strings.HasPrefix(name, "outpost-") {
					incusName = name
					break
				}
			}
		}
		opts := CreateOptions{}
		if s.project.Machine != nil {
			opts.Image = s.project.Machine.Image
			opts.CPU = s.project.Machine.CPU
			opts.VirtualMachine = s.project.Machine.VirtualMachine
			if s.project.Machine.Memory != "" {
				if v, parseErr := ParseSize(s.project.Machine.Memory); parseErr == nil {
					opts.MemoryBytes = v
				}
			}
			if s.project.Machine.Disk != "" {
				if v, parseErr := ParseSize(s.project.Machine.Disk); parseErr == nil {
					opts.DiskBytes = v
				}
			}
		}
		opts.ApplyDefaults()
		meta := Meta{
			Name:        config.SanitizeMachineName(s.name()),
			IncusName:   incusName,
			Type:        TypeContainer,
			Image:       opts.Image,
			CPU:         opts.CPU,
			MemoryBytes: opts.MemoryBytes,
			DiskBytes:   opts.DiskBytes,
		}
		if opts.VirtualMachine {
			meta.Type = TypeVM
		}
		if rt, err := s.service.listRuntime(ctx); err == nil {
			if info, ok := rt[meta.IncusName]; ok && strings.EqualFold(info.Type, "virtual-machine") {
				meta.Type = TypeVM
			}
		}
		remoteDir := s.service.machineRemoteDir(s.name())
		if err := transport.EnsureRemoteDir(s.service.Exec, remoteDir); err != nil {
			return err
		}
		metaBytes, err := yaml.Marshal(meta)
		if err != nil {
			return err
		}
		if err := s.service.Exec.UploadBytes(metaBytes, remoteDir+"/meta.yaml"); err != nil {
			return err
		}
	}
	_ = provider
	return nil
}

func normalizeProjectPath(spec, name string) string {
	if strings.HasPrefix(spec, "project:") {
		return name + ":" + strings.TrimPrefix(spec, "project:")
	}
	return spec
}
