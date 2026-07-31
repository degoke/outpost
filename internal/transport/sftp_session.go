package transport

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
)

// SFTPSession holds a reusable SFTP client for batched file operations.
type SFTPSession struct {
	client      *sftp.Client
	dirMu       sync.Mutex
	createdDirs map[string]bool
}

// ListFiles returns regular files below root, relative to root.
func (s *SFTPSession) ListFiles(root string) ([]string, error) {
	var files []string
	walker := s.client.Walk(root)
	for walker.Step() {
		if err := walker.Err(); err != nil {
			return nil, err
		}
		info := walker.Stat()
		if info == nil || !info.Mode().IsRegular() {
			continue
		}
		rel, err := filepath.Rel(root, walker.Path())
		if err != nil {
			return nil, err
		}
		files = append(files, filepath.ToSlash(rel))
	}
	return files, nil
}

// OpenSFTP opens a reusable SFTP session on the SSH connection.
func (e *SSHExecutor) OpenSFTP() (*SFTPSession, error) {
	client, err := e.connect()
	if err != nil {
		return nil, err
	}
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return nil, fmt.Errorf("sftp: %w", err)
	}
	return &SFTPSession{
		client:      sftpClient,
		createdDirs: map[string]bool{},
	}, nil
}

// Close closes the SFTP session.
func (s *SFTPSession) Close() error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Close()
}

// Stat returns remote file metadata.
func (s *SFTPSession) Stat(remote string) (os.FileInfo, error) {
	return s.client.Stat(remote)
}

// UploadWithProgress uploads a local file through the shared SFTP session.
func (s *SFTPSession) UploadWithProgress(local, remote string, out io.Writer) error {
	if err := s.EnsureRemoteDir(filepath.Dir(remote)); err != nil {
		return err
	}
	src, err := os.Open(local)
	if err != nil {
		return err
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return err
	}
	dst, err := s.client.Create(remote)
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = CopyWithProgress(dst, src, info.Size(), "Uploading to host", out, nil)
	return err
}

// HashRemote returns the SHA-256 hex digest of a remote file.
func (s *SFTPSession) HashRemote(remote string) (string, error) {
	f, err := s.client.Open(remote)
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

// Remove deletes a remote file through the shared SFTP session.
func (s *SFTPSession) Remove(remote string) error {
	return s.client.Remove(remote)
}

// EnsureRemoteDir creates a remote directory tree if needed.
func (s *SFTPSession) EnsureRemoteDir(dir string) error {
	dir = strings.ReplaceAll(dir, "\\", "/")
	if dir == "" || dir == "." {
		return nil
	}
	s.dirMu.Lock()
	defer s.dirMu.Unlock()
	if s.createdDirs[dir] {
		return nil
	}
	if err := ensureRemoteDir(s.client, dir); err != nil {
		return err
	}
	s.createdDirs[dir] = true
	return nil
}

// ModTimesMatch reports whether local and remote mtimes match at second precision.
func ModTimesMatch(local, remote os.FileInfo) bool {
	if local == nil || remote == nil {
		return false
	}
	lt := local.ModTime().UTC().Truncate(time.Second)
	rt := remote.ModTime().UTC().Truncate(time.Second)
	return lt.Equal(rt)
}

// Config returns the SSH connection settings.
func (e *SSHExecutor) Config() SSHConfig {
	return e.cfg
}

// Destination returns user@hostname for remote tools such as rsync.
func (e *SSHExecutor) Destination() string {
	return fmt.Sprintf("%s@%s", e.cfg.User, e.cfg.Hostname)
}

// RsyncSSHArgs returns ssh flags for rsync -e.
func (e *SSHExecutor) RsyncSSHArgs() []string {
	port := e.cfg.Port
	if port == 0 {
		port = 22
	}
	args := []string{"-p", fmt.Sprintf("%d", port)}
	if e.cfg.IdentityFile != "" {
		args = append(args, "-i", expandPath(e.cfg.IdentityFile))
	}
	if e.cfg.AutoTrustHostKey {
		args = append(args, "-o", "StrictHostKeyChecking=no")
	} else {
		args = append(args, "-o", "StrictHostKeyChecking=accept-new")
	}
	return args
}
