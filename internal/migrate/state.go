package migrate

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/transport"
)

const remoteStateArchive = "remote-outpost-state.tar.gz"

func exportRemoteState(ctx context.Context, exec transport.Executor, proj *config.Project) (bool, error) {
	staging := remoteMigrateStaging(proj)
	if err := transport.EnsureRemoteDir(exec, staging); err != nil {
		return false, err
	}
	remoteArchive := staging + "/" + remoteStateArchive
	stateRoot := strings.TrimRight(proj.RemoteDir, "/") + "/.outpost"
	cmd := fmt.Sprintf(
		"if [ -d %s ]; then tar czf %s --exclude='kubernetes' --exclude='machines' -C %s .outpost; else exit 0; fi",
		shellQuote(stateRoot),
		shellQuote(remoteArchive),
		shellQuote(strings.TrimRight(proj.RemoteDir, "/")),
	)
	code, err := exec.Run(ctx, cmd, transport.RunOpts{})
	if err != nil {
		return false, err
	}
	if code != 0 {
		return false, nil
	}
	return true, downloadArchiveIfExists(exec, proj.Name, remoteStateArchive, remoteArchive)
}

func downloadArchiveIfExists(exec transport.Executor, projectName, archiveName, remotePath string) error {
	data, err := exec.Download(remotePath)
	if err != nil {
		return nil
	}
	localPath, err := localArchiveFile(projectName, archiveName)
	if err != nil {
		return err
	}
	tmpPath := localPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmpPath, localPath)
}

func importRemoteState(ctx context.Context, exec transport.Executor, proj *config.Project) (bool, error) {
	localPath, err := localArchiveFile(proj.Name, remoteStateArchive)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(localPath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	staging := remoteMigrateStaging(proj)
	if err := transport.EnsureRemoteDir(exec, staging); err != nil {
		return false, err
	}
	data, err := os.ReadFile(localPath)
	if err != nil {
		return false, err
	}
	remoteArchive := staging + "/" + remoteStateArchive
	if err := exec.UploadBytes(data, remoteArchive); err != nil {
		return false, err
	}
	remoteDir := strings.TrimRight(proj.RemoteDir, "/")
	extractCmd := fmt.Sprintf(
		"mkdir -p %s/.outpost && tar xzf %s -C %s",
		shellQuote(remoteDir),
		shellQuote(remoteArchive),
		shellQuote(remoteDir),
	)
	code, err := exec.Run(ctx, extractCmd, transport.RunOpts{})
	if err != nil {
		return false, err
	}
	if code != 0 {
		return false, fmt.Errorf("extract remote outpost state failed (exit %d)", code)
	}
	return true, nil
}

func cleanupStaging(ctx context.Context, exec transport.Executor, proj *config.Project) {
	staging := remoteMigrateStaging(proj)
	_, _ = exec.Run(ctx, fmt.Sprintf("rm -rf %s", shellQuote(staging)), transport.RunOpts{})
}
