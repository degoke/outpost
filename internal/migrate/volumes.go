package migrate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/transport"
)

const migrateStagingDir = ".migrate-staging"

// DockerVolume describes a Docker volume to export/import during migration.
type DockerVolume struct {
	ArchiveName string `json:"archive_name"`
	DockerName  string `json:"docker_name"`
}

func remoteMigrateStaging(proj *config.Project) string {
	return strings.TrimRight(proj.RemoteDir, "/") + "/" + migrateStagingDir
}

func localArchiveFile(projectName, archiveName string) (string, error) {
	dir, err := config.VolumeArchivesDir(projectName)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, archiveName), nil
}

func exportDockerVolumesTo(ctx context.Context, exec transport.Executor, vols []DockerVolume, remoteStaging string) (int, error) {
	if len(vols) == 0 {
		return 0, nil
	}
	if err := transport.EnsureRemoteDir(exec, remoteStaging); err != nil {
		return 0, err
	}

	exported := 0
	for _, v := range vols {
		exists, err := dockerVolumeExists(ctx, exec, v.DockerName)
		if err != nil {
			return exported, err
		}
		if !exists {
			continue
		}
		empty, err := dockerVolumeEmpty(ctx, exec, v.DockerName)
		if err != nil {
			return exported, err
		}
		if empty {
			continue
		}
		exportCmd := fmt.Sprintf(
			"docker run --rm -v %s:/from:ro -v %s:/to alpine tar czf /to/%s -C /from .",
			shellQuote(v.DockerName),
			shellQuote(remoteStaging),
			shellQuote(v.ArchiveName),
		)
		code, err := exec.Run(ctx, exportCmd, transport.RunOpts{})
		if err != nil {
			return exported, err
		}
		if code != 0 {
			return exported, fmt.Errorf("export docker volume %s failed (exit %d)", v.DockerName, code)
		}
		exported++
	}
	return exported, nil
}

func importDockerVolumesFrom(ctx context.Context, exec transport.Executor, remoteStaging string, vols []DockerVolume, force bool) (int, error) {
	if len(vols) == 0 {
		return 0, nil
	}
	if err := transport.EnsureRemoteDir(exec, remoteStaging); err != nil {
		return 0, err
	}

	imported := 0
	for _, v := range vols {
		remoteArchive := remoteStaging + "/" + v.ArchiveName
		if !remoteFileExists(ctx, exec, remoteArchive) {
			continue
		}
		ok, err := importDockerVolumeFromRemote(ctx, exec, remoteStaging, v, force)
		if err != nil {
			return imported, err
		}
		if ok {
			imported++
		}
	}
	return imported, nil
}

func importDockerVolumeFromRemote(ctx context.Context, exec transport.Executor, staging string, v DockerVolume, force bool) (bool, error) {
	exists, err := dockerVolumeExists(ctx, exec, v.DockerName)
	if err != nil {
		return false, err
	}
	if exists && !force {
		empty, err := dockerVolumeEmpty(ctx, exec, v.DockerName)
		if err != nil {
			return false, err
		}
		if !empty {
			return false, nil
		}
	}
	if !exists {
		createCmd := fmt.Sprintf("docker volume create %s", shellQuote(v.DockerName))
		code, err := exec.Run(ctx, createCmd, transport.RunOpts{})
		if err != nil {
			return false, err
		}
		if code != 0 {
			return false, fmt.Errorf("create docker volume %s failed (exit %d)", v.DockerName, code)
		}
	}
	importCmd := fmt.Sprintf(
		"docker run --rm -v %s:/to -v %s:/from:ro alpine tar xzf /from/%s -C /to",
		shellQuote(v.DockerName),
		shellQuote(staging),
		shellQuote(v.ArchiveName),
	)
	code, err := exec.Run(ctx, importCmd, transport.RunOpts{})
	if err != nil {
		return false, err
	}
	if code != 0 {
		return false, fmt.Errorf("import docker volume %s failed (exit %d)", v.DockerName, code)
	}
	return true, nil
}

func dockerVolumeExists(ctx context.Context, exec transport.Executor, dockerName string) (bool, error) {
	cmd := fmt.Sprintf("docker volume inspect %s >/dev/null 2>&1", shellQuote(dockerName))
	code, err := exec.Run(ctx, cmd, transport.RunOpts{})
	if err != nil {
		return false, err
	}
	return code == 0, nil
}

func dockerVolumeEmpty(ctx context.Context, exec transport.Executor, dockerName string) (bool, error) {
	exists, err := dockerVolumeExists(ctx, exec, dockerName)
	if err != nil {
		return false, err
	}
	if !exists {
		return true, nil
	}
	cmd := fmt.Sprintf(
		"docker run --rm -v %s:/data:ro alpine sh -c 'if [ -z \"$(ls -A /data 2>/dev/null)\" ]; then exit 0; else exit 1; fi'",
		shellQuote(dockerName),
	)
	code, err := exec.Run(ctx, cmd, transport.RunOpts{})
	if err != nil {
		return false, err
	}
	return code == 0, nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func downloadArchive(exec transport.Executor, projectName, archiveName, remotePath string) error {
	data, err := exec.Download(remotePath)
	if err != nil {
		return err
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
