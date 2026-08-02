package upload

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/transport"
)

// PullRepo downloads the project repository from the remote host to the local
// working tree. It mirrors SyncRepo exclusions and never deletes local files.
func PullRepo(cwd string, proj *config.Project, exec transport.Executor, opts *SyncOpts) error {
	if ssh, ok := exec.(*transport.SSHExecutor); ok && (opts == nil || !opts.ForceSFTP) {
		if err := pullRepoRsync(cwd, proj, ssh, opts); err == nil {
			return nil
		} else if opts != nil && opts.UseRsync {
			return err
		} else if !isRsyncUnavailable(err) {
			return err
		}
	}
	return pullRepoSFTP(cwd, proj, exec, opts)
}

func pullRepoSFTP(cwd string, proj *config.Project, exec transport.Executor, opts *SyncOpts) error {
	ssh, ok := exec.(*transport.SSHExecutor)
	if !ok {
		return pullRepoLegacy(cwd, proj, exec, opts)
	}
	session, err := ssh.OpenSFTP()
	if err != nil {
		return pullRepoLegacy(cwd, proj, exec, opts)
	}
	defer session.Close()

	files, err := collectRemoteRepoPaths(cwd, proj, session)
	if err != nil {
		return err
	}
	return pullFiles(cwd, proj, session, files, opts)
}

func collectRemoteRepoPaths(cwd string, proj *config.Project, session *transport.SFTPSession) ([]string, error) {
	all, err := session.ListFiles(proj.RemoteDir)
	if err != nil {
		return nil, err
	}
	patterns := loadOutpostIgnore(cwd)
	gitRepo := isGitRepo(cwd)
	var paths []string
	for _, rel := range all {
		if shouldIgnoreRepo(rel, patterns, false) {
			continue
		}
		if gitRepo && gitIgnoredByGit(cwd, rel) {
			continue
		}
		paths = append(paths, rel)
	}
	return paths, nil
}

type pullTask struct {
	rel    string
	local  string
	remote string
}

func pullFiles(cwd string, proj *config.Project, session *transport.SFTPSession, files []string, opts *SyncOpts) error {
	if opts != nil && opts.Out != nil && !opts.Out.JSON {
		opts.Out.Step("Pulling changes from remote...")
	}

	tasks := make([]pullTask, 0, len(files))
	for _, rel := range files {
		if !safeRepoRelativePath(rel) {
			return fmt.Errorf("unsafe remote repository path %q", rel)
		}
		if err := ensureSafeLocalRepoPath(cwd, rel); err != nil {
			return err
		}
		local := filepath.Join(cwd, rel)
		remote := remotePath(proj, rel)
		needPull, err := needsPullSession(session, local, remote)
		if err != nil {
			return err
		}
		if !needPull {
			continue
		}
		tasks = append(tasks, pullTask{rel: rel, local: local, remote: remote})
	}
	if len(tasks) == 0 {
		if opts != nil && opts.Out != nil && !opts.Out.JSON {
			opts.Out.Step("Local files already up to date")
		}
		return nil
	}

	workers := opts.workers()
	if workers > len(tasks) {
		workers = len(tasks)
	}

	var (
		mu       sync.Mutex
		pulled   int
		firstErr error
		wg       sync.WaitGroup
		sem      = make(chan struct{}, workers)
	)

	for _, task := range tasks {
		wg.Add(1)
		go func(task pullTask) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if err := os.MkdirAll(filepath.Dir(task.local), 0755); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return
			}
			if err := session.DownloadWithProgress(task.remote, task.local, nil); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return
			}

			mu.Lock()
			pulled++
			if opts != nil && opts.Out != nil && !opts.Out.JSON {
				opts.Out.Step("Pulled %s (%d/%d)...", task.rel, pulled, len(tasks))
			}
			mu.Unlock()
		}(task)
	}
	wg.Wait()
	return firstErr
}

func needsPullSession(session *transport.SFTPSession, local, remote string) (bool, error) {
	remoteInfo, err := session.Stat(remote)
	if err != nil {
		return false, nil
	}
	localInfo, err := os.Stat(local)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return true, err
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

func listRemoteRepoPaths(cwd string, proj *config.Project, exec transport.Executor) ([]string, error) {
	cmd := fmt.Sprintf("find %s -type f -printf '%%P\\n' 2>/dev/null || true", shellQuotePath(proj.RemoteDir))
	var stdout strings.Builder
	code, err := exec.Run(context.Background(), cmd, transport.RunOpts{Stdout: &stdout})
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("list remote files failed (exit %d)", code)
	}
	patterns := loadOutpostIgnore(cwd)
	gitRepo := isGitRepo(cwd)
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		rel := filepath.ToSlash(strings.TrimSpace(line))
		if rel == "" {
			continue
		}
		if !safeRepoRelativePath(rel) {
			return nil, fmt.Errorf("unsafe remote repository path %q", rel)
		}
		if shouldIgnoreRepo(rel, patterns, false) {
			continue
		}
		if gitRepo && gitIgnoredByGit(cwd, rel) {
			continue
		}
		paths = append(paths, rel)
	}
	return paths, nil
}

func shellQuotePath(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func pullRepoLegacy(cwd string, proj *config.Project, exec transport.Executor, opts *SyncOpts) error {
	localPaths, err := collectRepoPaths(cwd)
	if err != nil {
		return err
	}
	seen := make(map[string]bool, len(localPaths))
	for _, rel := range localPaths {
		seen[rel] = true
	}
	// Include remote-only files when the executor can enumerate them.
	if remotePaths, err := listRemoteRepoPaths(cwd, proj, exec); err == nil {
		for _, rel := range remotePaths {
			if !safeRepoRelativePath(rel) {
				return fmt.Errorf("unsafe remote repository path %q", rel)
			}
			if !seen[rel] {
				localPaths = append(localPaths, rel)
				seen[rel] = true
			}
		}
	}
	if opts != nil && opts.Out != nil && !opts.Out.JSON {
		opts.Out.Step("Pulling changes from remote...")
	}
	pulled := 0
	for _, rel := range localPaths {
		if err := ensureSafeLocalRepoPath(cwd, rel); err != nil {
			return err
		}
		local := filepath.Join(cwd, rel)
		remote := remotePath(proj, rel)
		data, err := exec.Download(remote)
		if err != nil {
			continue
		}
		needPull, err := needsPullBytes(local, data)
		if err != nil {
			return err
		}
		if !needPull {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(local), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(local, data, 0644); err != nil {
			return err
		}
		pulled++
		if opts != nil && opts.Out != nil && !opts.Out.JSON {
			opts.Out.Step("Pulled %s (%d)...", rel, pulled)
		}
	}
	if opts != nil && opts.Out != nil && !opts.Out.JSON && pulled == 0 {
		opts.Out.Step("Local files already up to date")
	}
	return nil
}

func safeRepoRelativePath(rel string) bool {
	if rel == "" || filepath.IsAbs(rel) {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(rel))
	return clean != ".." && !strings.HasPrefix(clean, "../")
}

// ensureSafeLocalRepoPath prevents a remote repository pull from following
// symlinks already present in the local checkout. A remote-controlled path
// must not be able to redirect a write outside the selected working tree.
func ensureSafeLocalRepoPath(cwd, rel string) error {
	if !safeRepoRelativePath(rel) {
		return fmt.Errorf("unsafe repository path %q", rel)
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	current := cwd
	for _, part := range strings.Split(clean, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("inspect local path %q: %w", filepath.Join(rel), err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to write through local symlink %q", filepath.Join(rel))
		}
	}
	return nil
}

// EnsureSafeLocalRepoPathForTest exposes local pull path validation to tests.
func EnsureSafeLocalRepoPathForTest(cwd, rel string) error {
	return ensureSafeLocalRepoPath(cwd, rel)
}

func needsPullBytes(local string, remote []byte) (bool, error) {
	localInfo, err := os.Stat(local)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return true, err
	}
	if int64(len(remote)) != localInfo.Size() {
		return true, nil
	}
	localHash, err := hashLocalFile(local)
	if err != nil {
		return true, err
	}
	remoteHash := hashBytes(remote)
	return localHash != remoteHash, nil
}

func hashBytes(data []byte) string {
	h := sha256.New()
	_, _ = h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// CollectRemoteRepoPathsForTest exposes remote repo path collection for tests.
func CollectRemoteRepoPathsForTest(cwd string, proj *config.Project, session *transport.SFTPSession) ([]string, error) {
	return collectRemoteRepoPaths(cwd, proj, session)
}
