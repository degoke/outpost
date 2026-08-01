package migrate

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/degoke/outpost/internal/bootstrap"
	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/machine"
	"github.com/degoke/outpost/internal/output"
	"github.com/degoke/outpost/internal/transport"
)

const (
	machineArchiveName     = "machine.tar.gz"
	machineMetaArchiveName = "machine-meta.tar.gz"
)

func exportMachine(ctx context.Context, exec transport.Executor, proj *config.Project, out *output.Printer) (bool, error) {
	if proj.Machine == nil {
		return false, nil
	}
	if err := bootstrap.EnsureIncus(ctx, exec); err != nil {
		return false, err
	}
	svc := machine.NewProjectService(exec, proj, out)
	if !svc.Exists(ctx) {
		return false, nil
	}
	staging := remoteMigrateStaging(proj)
	if err := transport.EnsureRemoteDir(exec, staging); err != nil {
		return false, err
	}
	if out != nil {
		out.Step("Exporting project Incus machine...")
	}
	remoteArchive := staging + "/" + machineArchiveName
	if err := svc.ExportArchive(ctx, remoteArchive); err != nil {
		return false, err
	}
	if err := downloadArchive(exec, proj.Name, machineArchiveName, remoteArchive); err != nil {
		return false, err
	}

	metaDir := strings.TrimRight(proj.RemoteDir, "/") + "/.outpost/machines/" + config.SanitizeMachineName(proj.Name)
	remoteMeta := staging + "/" + machineMetaArchiveName
	metaCmd := fmt.Sprintf(
		"if [ -d %s ]; then tar czf %s -C %s .; fi",
		shellQuote(metaDir),
		shellQuote(remoteMeta),
		shellQuote(metaDir),
	)
	if _, err := exec.Run(ctx, metaCmd, transport.RunOpts{}); err != nil {
		return false, err
	}
	return true, downloadArchiveIfExists(exec, proj.Name, machineMetaArchiveName, remoteMeta)
}

func importMachine(ctx context.Context, exec transport.Executor, proj *config.Project, provider *config.ProviderMeta, out *output.Printer) (bool, error) {
	if proj.Machine == nil {
		return false, nil
	}
	localPath, err := localArchiveFile(proj.Name, machineArchiveName)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(localPath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if err := bootstrap.EnsureIncus(ctx, exec); err != nil {
		return false, err
	}
	svc := machine.NewProjectService(exec, proj, out)
	if svc.Exists(ctx) {
		if out != nil {
			out.Info("Project Incus machine already exists on destination host")
		}
		return false, nil
	}
	staging := remoteMigrateStaging(proj)
	if err := transport.EnsureRemoteDir(exec, staging); err != nil {
		return false, err
	}

	if err := importMachineMetadata(ctx, exec, proj, staging); err != nil {
		return false, err
	}

	data, err := os.ReadFile(localPath)
	if err != nil {
		return false, err
	}
	remoteArchive := staging + "/" + machineArchiveName
	if err := exec.UploadBytes(data, remoteArchive); err != nil {
		return false, err
	}
	if out != nil {
		out.Step("Importing project Incus machine...")
	}
	if err := svc.ImportArchive(ctx, remoteArchive, provider); err != nil {
		return false, err
	}
	return true, nil
}

func importMachineMetadata(ctx context.Context, exec transport.Executor, proj *config.Project, staging string) error {
	metaPath, err := localArchiveFile(proj.Name, machineMetaArchiveName)
	if err != nil {
		return err
	}
	if _, err := os.Stat(metaPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		return err
	}
	remoteMeta := staging + "/" + machineMetaArchiveName
	if err := exec.UploadBytes(metaData, remoteMeta); err != nil {
		return err
	}
	metaDir := strings.TrimRight(proj.RemoteDir, "/") + "/.outpost/machines/" + config.SanitizeMachineName(proj.Name)
	extractMeta := fmt.Sprintf(
		"mkdir -p %s && tar xzf %s -C %s",
		shellQuote(metaDir),
		shellQuote(remoteMeta),
		shellQuote(metaDir),
	)
	code, err := exec.Run(ctx, extractMeta, transport.RunOpts{})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("extract machine metadata failed (exit %d)", code)
	}
	return nil
}
