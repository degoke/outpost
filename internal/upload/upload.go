package upload

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/transport"
)

func SyncProject(cwd string, proj *config.Project, exec transport.Executor) error {
	if err := transport.EnsureRemoteDir(exec, proj.RemoteDir); err != nil {
		return err
	}
	files, err := collectSyncPaths(cwd, proj)
	if err != nil {
		return err
	}
	for _, rel := range files {
		local := filepath.Join(cwd, rel)
		remote := remotePath(proj, rel)
		if err := syncFile(exec, local, remote); err != nil {
			return err
		}
	}
	return nil
}

func syncFile(exec transport.Executor, local, remote string) error {
	needUpload, err := needsUpload(exec, local, remote)
	if err != nil {
		return err
	}
	if !needUpload {
		return nil
	}
	return exec.Upload(local, remote)
}

func needsUpload(exec transport.Executor, local, remote string) (bool, error) {
	return NeedsUpload(exec, local, remote)
}

// NeedsUpload reports whether local and remote file content differ.
func NeedsUpload(exec transport.Executor, local, remote string) (bool, error) {
	localHash, err := fileHash(local)
	if err != nil {
		return true, err
	}
	remoteData, err := exec.Download(remote)
	if err != nil {
		return true, nil
	}
	remoteHash := sha256.Sum256(remoteData)
	if hex.EncodeToString(remoteHash[:]) == localHash {
		return false, nil
	}
	return true, nil
}

func fileHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func RemoteComposeArgs(proj *config.Project) string {
	var parts []string
	for _, f := range allComposeFiles(proj) {
		parts = append(parts, "-f", proj.RemoteDir+"/"+filepath.Base(f))
	}
	return strings.Join(parts, " ")
}

func allComposeFiles(proj *config.Project) []string {
	files := append([]string{}, proj.ComposeFiles...)
	files = append(files, proj.ExtraFiles...)
	return files
}

// AllComposeFiles returns compose and override files for a project.
func AllComposeFiles(proj *config.Project) []string {
	return allComposeFiles(proj)
}
