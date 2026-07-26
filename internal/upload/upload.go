package upload

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goke/outpost/internal/config"
	"github.com/goke/outpost/internal/transport"
)

func SyncProject(cwd string, proj *config.Project, exec transport.Executor) error {
	if err := transport.EnsureRemoteDir(exec, proj.RemoteDir); err != nil {
		return err
	}
	extra := []string{}
	if _, err := os.Stat(filepath.Join(cwd, ".env")); err == nil {
		extra = append(extra, ".env")
	}
	files := append([]string{}, proj.ComposeFiles...)
	files = append(files, proj.ExtraFiles...)
	files = append(files, extra...)

	for _, rel := range files {
		local := filepath.Join(cwd, rel)
		if _, err := os.Stat(local); err != nil {
			return fmt.Errorf("local file %s: %w", rel, err)
		}
		remote := proj.RemoteDir + "/" + filepath.Base(rel)
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
	for _, f := range proj.ComposeFiles {
		parts = append(parts, "-f", proj.RemoteDir+"/"+filepath.Base(f))
	}
	return strings.Join(parts, " ")
}

