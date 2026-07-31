package upload

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/output"
	"github.com/degoke/outpost/internal/transport"
)

const DefaultSyncWorkers = 6

// SyncOpts configures file sync behavior.
type SyncOpts struct {
	Out       *output.Printer
	Workers   int
	UseRsync  bool
	ForceSFTP bool
}

func (o *SyncOpts) progressWriter() io.Writer {
	if o == nil || o.Out == nil || o.Out.JSON {
		return nil
	}
	return o.Out.Stderr
}

func (o *SyncOpts) workers() int {
	if o == nil || o.Workers <= 0 {
		return DefaultSyncWorkers
	}
	return o.Workers
}

type uploadTask struct {
	rel    string
	local  string
	remote string
}

func syncFiles(cwd string, proj *config.Project, exec transport.Executor, files []string, opts *SyncOpts) error {
	ssh, ok := exec.(*transport.SSHExecutor)
	if !ok {
		return syncFilesLegacy(cwd, proj, exec, files, opts)
	}

	session, err := ssh.OpenSFTP()
	if err != nil {
		return syncFilesLegacy(cwd, proj, exec, files, opts)
	}
	defer session.Close()

	if opts != nil && opts.Out != nil && !opts.Out.JSON {
		opts.Out.Step("Syncing %d files...", len(files))
	}

	tasks := make([]uploadTask, 0, len(files))
	for _, rel := range files {
		local := filepath.Join(cwd, rel)
		if _, err := os.Stat(local); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		remote := remotePath(proj, rel)
		needUpload, err := needsUploadSession(session, local, remote)
		if err != nil {
			return err
		}
		if !needUpload {
			continue
		}
		tasks = append(tasks, uploadTask{rel: rel, local: local, remote: remote})
	}

	if len(tasks) == 0 {
		if opts != nil && opts.Out != nil && !opts.Out.JSON {
			opts.Out.Step("All files up to date")
		}
		return nil
	}

	remoteDirs := make(map[string]struct{})
	for _, task := range tasks {
		remoteDirs[filepath.Dir(task.remote)] = struct{}{}
	}
	for dir := range remoteDirs {
		if err := session.EnsureRemoteDir(dir); err != nil {
			return err
		}
	}

	workers := opts.workers()
	if workers > len(tasks) {
		workers = len(tasks)
	}

	var (
		mu       sync.Mutex
		uploaded int
		firstErr error
		wg       sync.WaitGroup
		sem      = make(chan struct{}, workers)
	)

	for _, task := range tasks {
		wg.Add(1)
		go func(task uploadTask) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			// Byte-level progress is disabled during parallel uploads to avoid stderr races.
			if err := session.UploadWithProgress(task.local, task.remote, nil); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return
			}

			mu.Lock()
			uploaded++
			if opts != nil && opts.Out != nil && !opts.Out.JSON {
				opts.Out.Step("Uploaded %s (%d/%d)...", task.rel, uploaded, len(tasks))
			}
			mu.Unlock()
		}(task)
	}
	wg.Wait()
	return firstErr
}

func syncFilesLegacy(cwd string, proj *config.Project, exec transport.Executor, files []string, opts *SyncOpts) error {
	if opts != nil && opts.Out != nil && !opts.Out.JSON {
		opts.Out.Step("Syncing %d files...", len(files))
	}
	progress := opts.progressWriter()
	uploaded := 0
	for _, rel := range files {
		local := filepath.Join(cwd, rel)
		if _, err := os.Stat(local); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		remote := remotePath(proj, rel)
		needUpload, err := needsUpload(exec, local, remote)
		if err != nil {
			return err
		}
		if !needUpload {
			continue
		}
		uploaded++
		if opts != nil && opts.Out != nil && !opts.Out.JSON {
			opts.Out.Step("Uploading %s (%d/%d)...", rel, uploaded, len(files))
		}
		if err := UploadFile(exec, local, remote, progress); err != nil {
			return err
		}
	}
	if opts != nil && opts.Out != nil && !opts.Out.JSON && uploaded == 0 {
		opts.Out.Step("All files up to date")
	}
	return nil
}

func deleteRemoteFiles(proj *config.Project, exec transport.Executor, relPaths []string) error {
	if len(relPaths) == 0 {
		return nil
	}
	ssh, ok := exec.(*transport.SSHExecutor)
	if !ok {
		return nil
	}
	session, err := ssh.OpenSFTP()
	if err != nil {
		return err
	}
	defer session.Close()

	for _, rel := range relPaths {
		remote := remotePath(proj, rel)
		if err := session.Remove(remote); err != nil {
			if isNotExistErr(err) {
				continue
			}
			return fmt.Errorf("delete remote %s: %w", rel, err)
		}
	}
	return nil
}

func isNotExistErr(err error) bool {
	return os.IsNotExist(err) || strings.Contains(err.Error(), "file does not exist") || strings.Contains(err.Error(), "no such file")
}

func syncFileChanges(cwd string, proj *config.Project, exec transport.Executor, files []string, opts *SyncOpts) error {
	var uploads, deletes []string
	for _, rel := range files {
		local := filepath.Join(cwd, rel)
		if _, err := os.Stat(local); err != nil {
			if os.IsNotExist(err) {
				deletes = append(deletes, rel)
				continue
			}
			return err
		}
		uploads = append(uploads, rel)
	}
	if len(deletes) > 0 {
		if err := deleteRemoteFiles(proj, exec, deletes); err != nil {
			return err
		}
		if opts != nil && opts.Out != nil && !opts.Out.JSON {
			opts.Out.Step("Removed %d deleted file(s) from remote", len(deletes))
		}
	}
	if len(uploads) == 0 {
		return nil
	}
	return syncFiles(cwd, proj, exec, uploads, opts)
}

func SyncProject(cwd string, proj *config.Project, exec transport.Executor, opts *SyncOpts) error {
	if err := transport.EnsureRemoteDir(exec, proj.RemoteDir); err != nil {
		return err
	}
	files, err := collectSyncPaths(cwd, proj)
	if err != nil {
		return err
	}
	return syncFiles(cwd, proj, exec, files, opts)
}

func SyncRepo(cwd string, proj *config.Project, exec transport.Executor, opts *SyncOpts) error {
	if ssh, ok := exec.(*transport.SSHExecutor); ok && (opts == nil || (!opts.ForceSFTP)) {
		if err := syncRepoRsync(cwd, proj, ssh, opts); err == nil {
			return reconcileRemoteFiles(cwd, proj, ssh)
		} else if opts != nil && opts.UseRsync {
			return err
		} else if !isRsyncUnavailable(err) {
			return err
		}
	}
	if err := transport.EnsureRemoteDir(exec, proj.RemoteDir); err != nil {
		return err
	}
	files, err := collectRepoPaths(cwd)
	if err != nil {
		return err
	}
	if err := syncFiles(cwd, proj, exec, files, opts); err != nil {
		return err
	}
	if ssh, ok := exec.(*transport.SSHExecutor); ok {
		return reconcileRemoteFiles(cwd, proj, ssh)
	}
	return nil
}

func isRsyncUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "rsync not found") || strings.Contains(msg, "password-only auth") || strings.Contains(msg, "requires SSH key")
}

func reconcileRemoteFiles(cwd string, proj *config.Project, ssh *transport.SSHExecutor) error {
	// Delete only files recorded by an earlier Outpost sync. This avoids
	// destroying files generated by commands inside the remote environment.
	local, err := collectRepoPaths(cwd)
	if err != nil {
		return err
	}
	keep := make(map[string]bool, len(local))
	for _, rel := range local {
		keep[filepath.ToSlash(rel)] = true
	}
	manifestPath := filepath.Join(proj.RemoteDir, ".outpost", "managed-files")
	previousData, _ := ssh.Download(manifestPath)
	previous := map[string]bool{}
	for _, rel := range strings.Split(string(previousData), "\n") {
		rel = filepath.ToSlash(strings.TrimSpace(rel))
		if rel != "" {
			previous[rel] = true
		}
	}
	session, err := ssh.OpenSFTP()
	if err != nil {
		return err
	}
	defer session.Close()
	remote, err := session.ListFiles(proj.RemoteDir)
	if err != nil {
		return err
	}
	for _, rel := range remote {
		if keep[rel] || !previous[rel] {
			continue
		}
		if err := session.Remove(filepath.Join(proj.RemoteDir, rel)); err != nil && !isNotExistErr(err) {
			return fmt.Errorf("remove stale remote file %s: %w", rel, err)
		}
	}
	sort.Strings(local)
	manifest := strings.Join(local, "\n") + "\n"
	if err := ssh.UploadBytes([]byte(manifest), manifestPath); err != nil {
		return fmt.Errorf("write remote sync manifest: %w", err)
	}
	return nil
}

// SyncPaths uploads or removes specific repository-relative paths.
func SyncPaths(cwd string, proj *config.Project, exec transport.Executor, relPaths []string, opts *SyncOpts) error {
	if len(relPaths) == 0 {
		return nil
	}
	if opts != nil && opts.UseRsync {
		return fmt.Errorf("rsync mode does not support partial path sync; use a full repository sync instead")
	}
	if err := transport.EnsureRemoteDir(exec, proj.RemoteDir); err != nil {
		return err
	}
	return syncFileChanges(cwd, proj, exec, relPaths, opts)
}
