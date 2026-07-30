package machine

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/transport"
	"github.com/degoke/outpost/internal/upload"
)

type copyEndpoint struct {
	Machine string
	Path    string
}

func parseCopyEndpoint(spec string) (copyEndpoint, bool, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return copyEndpoint{}, false, fmt.Errorf("path is required")
	}
	if len(spec) >= 2 && spec[1] == ':' && spec[0] >= 'A' && spec[0] <= 'Z' {
		return copyEndpoint{Path: spec}, false, nil
	}
	idx := strings.Index(spec, ":")
	if idx <= 0 {
		return copyEndpoint{Path: spec}, false, nil
	}
	path := spec[idx+1:]
	if path == "" {
		return copyEndpoint{}, false, fmt.Errorf("invalid machine path %q", spec)
	}
	name := config.SanitizeMachineName(spec[:idx])
	if name == "" {
		return copyEndpoint{}, false, fmt.Errorf("invalid machine name in %q", spec)
	}
	return copyEndpoint{Machine: name, Path: path}, true, nil
}

// ParseCopyEndpointForTest exposes copy path parsing for tests.
func ParseCopyEndpointForTest(spec string) (copyEndpoint, bool, error) {
	return parseCopyEndpoint(spec)
}

func expandLocalPath(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return filepath.Join(home, p[2:])
	}
	return p
}

// ExpandLocalPathForTest exposes local path expansion for tests.
func ExpandLocalPathForTest(p string) string {
	return expandLocalPath(p)
}

func (s *Service) Copy(ctx context.Context, src, dst string, recursive bool) error {
	srcEP, srcMachine, err := parseCopyEndpoint(src)
	if err != nil {
		return err
	}
	dstEP, dstMachine, err := parseCopyEndpoint(dst)
	if err != nil {
		return err
	}
	switch {
	case srcMachine && dstMachine:
		return fmt.Errorf("copy between two machines is not supported — copy through your computer")
	case srcMachine:
		return s.copyFromMachine(ctx, srcEP.Machine, srcEP.Path, expandLocalPath(dstEP.Path), recursive)
	case dstMachine:
		return s.copyToMachine(ctx, expandLocalPath(srcEP.Path), dstEP.Machine, dstEP.Path, recursive)
	default:
		return fmt.Errorf("copy requires one local path and one machine path (NAME:/path)")
	}
}

func (s *Service) copyToMachine(ctx context.Context, localPath, machineName, remotePath string, recursive bool) error {
	incusName, err := s.resolveIncusName(ctx, machineName)
	if err != nil {
		return err
	}
	info, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("open local path: %w", err)
	}
	if info.IsDir() && !recursive {
		return fmt.Errorf("%s is a directory — use -r to copy recursively", localPath)
	}
	if err := s.ensureRemoteParentDir(ctx, incusName, remotePath); err != nil {
		return err
	}

	stagingDir := RemoteDir(machineName) + "/.outpost/staging"
	if err := transport.EnsureRemoteDir(s.Exec, stagingDir); err != nil {
		return err
	}
	stagingPath := stagingDir + "/" + filepath.Base(localPath)
	if err := upload.UploadFile(s.Exec, localPath, stagingPath, s.copyProgressWriter()); err != nil {
		return fmt.Errorf("upload to host: %w", err)
	}
	defer func() { _, _ = s.Exec.Run(ctx, "rm -rf "+shellQuote(stagingPath), quietRunOpts()) }()

	pushParts := []string{"file push"}
	if !s.copyProgressEnabled() {
		pushParts = append(pushParts, "--quiet")
	}
	if recursive || info.IsDir() {
		pushParts = append(pushParts, "-r")
	}
	pushParts = append(pushParts,
		shellQuote(stagingPath),
		shellQuote(incusInstancePath(incusName, remotePath)),
	)
	pushCmd, err := s.incusCommand(ctx, strings.Join(pushParts, " "))
	if err != nil {
		return err
	}
	code, err := s.Exec.Run(ctx, pushCmd, s.copyRunOpts())
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("incus file push failed (exit %d)", code)
	}
	if !info.IsDir() && info.Mode()&0111 != 0 {
		chmodCmd, err := s.incusCommand(ctx, fmt.Sprintf("exec %s -- chmod +x %s", shellQuote(incusName), shellQuote(remotePath)))
		if err == nil {
			_, _ = s.Exec.Run(ctx, chmodCmd, quietRunOpts())
		}
	}
	if s.Out != nil && !s.Out.JSON {
		s.Out.Success("Copied %s -> %s:%s", localPath, machineName, remotePath)
	}
	return nil
}

func (s *Service) copyFromMachine(ctx context.Context, machineName, remotePath, localPath string, recursive bool) error {
	incusName, err := s.resolveIncusName(ctx, machineName)
	if err != nil {
		return err
	}
	stagingDir := RemoteDir(machineName) + "/.outpost/staging"
	if err := transport.EnsureRemoteDir(s.Exec, stagingDir); err != nil {
		return err
	}
	stagingPath := stagingDir + "/" + filepath.Base(remotePath)
	defer func() { _, _ = s.Exec.Run(ctx, "rm -rf "+shellQuote(stagingPath), quietRunOpts()) }()

	pullParts := []string{"file pull"}
	if !s.copyProgressEnabled() {
		pullParts = append(pullParts, "--quiet")
	}
	if recursive {
		pullParts = append(pullParts, "-r")
	}
	pullParts = append(pullParts,
		shellQuote(incusInstancePath(incusName, remotePath)),
		shellQuote(stagingPath),
	)
	pullCmd, err := s.incusCommand(ctx, strings.Join(pullParts, " "))
	if err != nil {
		return err
	}
	code, err := s.Exec.Run(ctx, pullCmd, s.copyRunOpts())
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("incus file pull failed (exit %d)", code)
	}
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return err
	}
	if err := downloadToLocal(s.Exec, stagingPath, localPath, s.copyProgressWriter()); err != nil {
		return fmt.Errorf("download from host: %w", err)
	}
	if s.Out != nil && !s.Out.JSON {
		s.Out.Success("Copied %s:%s -> %s", machineName, remotePath, localPath)
	}
	return nil
}

func quietRunOpts() transport.RunOpts {
	return transport.RunOpts{Stdout: io.Discard, Stderr: io.Discard}
}

func (s *Service) copyProgressEnabled() bool {
	return s.Out != nil && !s.Out.JSON
}

func (s *Service) copyProgressWriter() io.Writer {
	if s.copyProgressEnabled() {
		return s.Out.Stderr
	}
	return nil
}

func (s *Service) copyRunOpts() transport.RunOpts {
	if s.copyProgressEnabled() {
		return transport.RunOpts{Stdout: io.Discard, Stderr: s.Out.Stderr}
	}
	return quietRunOpts()
}

func downloadToLocal(exec transport.Executor, remotePath, localPath string, progress io.Writer) error {
	if progress != nil {
		if downloader, ok := exec.(interface {
			DownloadToWithProgress(local, remote string, out io.Writer) error
		}); ok {
			return downloader.DownloadToWithProgress(localPath, remotePath, progress)
		}
	}
	if downloader, ok := exec.(interface {
		DownloadTo(local, remote string) error
	}); ok {
		return downloader.DownloadTo(localPath, remotePath)
	}
	data, err := exec.Download(remotePath)
	if err != nil {
		return err
	}
	return os.WriteFile(localPath, data, 0644)
}

func (s *Service) ensureRemoteParentDir(ctx context.Context, incusName, remotePath string) error {
	parent := path.Dir(remotePath)
	if parent == "." || parent == "/" || parent == "" {
		return nil
	}
	cmd, err := s.incusCommand(ctx, fmt.Sprintf("exec %s -- mkdir -p %s", shellQuote(incusName), shellQuote(parent)))
	if err != nil {
		return err
	}
	code, err := s.Exec.Run(ctx, cmd, quietRunOpts())
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("create remote directory %s failed (exit %d)", parent, code)
	}
	return nil
}
