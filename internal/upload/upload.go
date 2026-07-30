package upload

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/transport"
)

func needsUpload(exec transport.Executor, local, remote string) (bool, error) {
	return NeedsUpload(exec, local, remote)
}

// NeedsUpload reports whether local and remote file content differ.
func NeedsUpload(exec transport.Executor, local, remote string) (bool, error) {
	if ssh, ok := exec.(*transport.SSHExecutor); ok {
		session, err := ssh.OpenSFTP()
		if err == nil {
			defer session.Close()
			return needsUploadSession(session, local, remote)
		}
	}
	return needsUploadLegacy(exec, local, remote)
}

func needsUploadSession(session *transport.SFTPSession, local, remote string) (bool, error) {
	localInfo, err := os.Stat(local)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return true, err
	}
	remoteInfo, err := session.Stat(remote)
	if err != nil {
		return true, nil
	}
	if localInfo.Size() != remoteInfo.Size() {
		return true, nil
	}
	if transport.ModTimesMatch(localInfo, remoteInfo) {
		return false, nil
	}
	localHash, err := hashLocalFile(local)
	if err != nil {
		return true, err
	}
	remoteHash, err := session.HashRemote(remote)
	if err != nil {
		return true, nil
	}
	return localHash != remoteHash, nil
}

func needsUploadLegacy(exec transport.Executor, local, remote string) (bool, error) {
	localHash, err := hashLocalFile(local)
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

func hashLocalFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ModTimesEqual is exported for tests.
func ModTimesEqual(local, remote os.FileInfo) bool {
	return transport.ModTimesMatch(local, remote)
}

// TruncateToSecond truncates a time to second precision in UTC.
func TruncateToSecond(t time.Time) time.Time {
	return t.UTC().Truncate(time.Second)
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
