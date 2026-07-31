package mirror

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/degoke/outpost/internal/upload"
	"github.com/fsnotify/fsnotify"
)

type SyncOptions struct {
	UseRsync  bool
	ForceSFTP bool
	Workers   int
}

type WatchOptions struct {
	SyncOptions
	Debounce        time.Duration
	SkipInitialSync bool
}

const defaultWatchDebounce = time.Second

func (r *Runner) syncOptsFrom(opts SyncOptions) *upload.SyncOpts {
	base := r.syncOpts()
	out := &upload.SyncOpts{}
	if base != nil {
		*out = *base
	}
	out.UseRsync = opts.UseRsync
	out.ForceSFTP = opts.ForceSFTP
	if opts.Workers > 0 {
		out.Workers = opts.Workers
	}
	return out
}

func (r *Runner) SyncWith(ctx context.Context, opts SyncOptions) error {
	return upload.SyncRepo(r.Cwd, r.Proj, r.Exec, r.syncOptsFrom(opts))
}

func (r *Runner) Watch(ctx context.Context, opts WatchOptions) error {
	release, err := acquireWatchLock(r.Host, r.Proj.Name, r.Cwd)
	if err != nil {
		return err
	}
	defer release()

	debounce := opts.Debounce
	if debounce <= 0 {
		debounce = defaultWatchDebounce
	}
	syncOpts := r.syncOptsFrom(opts.SyncOptions)

	if !opts.SkipInitialSync {
		if r.Out != nil {
			r.Out.Step("Performing initial sync...")
		}
		if err := upload.SyncRepo(r.Cwd, r.Proj, r.Exec, syncOpts); err != nil {
			return err
		}
		if err := r.recordSynced(); err != nil {
			return err
		}
	}
	if opts.UseRsync || !opts.ForceSFTP {
		if r.Out != nil {
			r.Out.Success("Watching for changes (rsync mode). Press Ctrl+C to stop.")
		}
		return r.watchRsync(ctx, debounce, syncOpts)
	}
	return r.watchSFTP(ctx, debounce, syncOpts)
}

type debouncer struct {
	delay   time.Duration
	mu      sync.Mutex
	timer   *time.Timer
	trigger chan struct{}
}

func newDebouncer(delay time.Duration) *debouncer {
	d := &debouncer{
		delay:   delay,
		trigger: make(chan struct{}, 1),
	}
	return d
}

func (d *debouncer) Notify() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.timer != nil {
		d.timer.Stop()
	}
	d.timer = time.AfterFunc(d.delay, func() {
		select {
		case d.trigger <- struct{}{}:
		default:
		}
	})
}

func (d *debouncer) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
}

func (r *Runner) watchRsync(ctx context.Context, debounce time.Duration, syncOpts *upload.SyncOpts) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	if err := addRepoWatches(watcher, r.Cwd); err != nil {
		return err
	}

	db := newDebouncer(debounce)
	defer db.Stop()

	var pending bool
	runSync := func() error {
		pending = false
		if r.Out != nil {
			r.Out.Step("Syncing changes...")
		}
		if err := upload.SyncRepo(r.Cwd, r.Proj, r.Exec, syncOpts); err != nil {
			return err
		}
		return r.recordSynced()
	}

	for {
		select {
		case <-ctx.Done():
			if pending {
				return runSync()
			}
			return nil
		case event, ok := <-watcher.Events:
			if !ok {
				if pending {
					return runSync()
				}
				return nil
			}
			if isIgnorableWatchEvent(event) {
				continue
			}
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					_ = addRepoWatches(watcher, event.Name)
				}
			}
			if shouldWatchPath(r.Cwd, event.Name) {
				pending = true
				db.Notify()
			}
		case <-db.trigger:
			if err := runSync(); err != nil {
				return err
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				if pending {
					return runSync()
				}
				return nil
			}
			return err
		}
	}
}

func (r *Runner) watchSFTP(ctx context.Context, debounce time.Duration, syncOpts *upload.SyncOpts) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	if err := addRepoWatches(watcher, r.Cwd); err != nil {
		return err
	}

	var (
		pendingMu sync.Mutex
		pending   = map[string]struct{}{}
	)
	db := newDebouncer(debounce)
	defer db.Stop()

	flush := func() error {
		pendingMu.Lock()
		paths := make([]string, 0, len(pending))
		for rel := range pending {
			paths = append(paths, rel)
		}
		pending = map[string]struct{}{}
		pendingMu.Unlock()
		if len(paths) == 0 {
			return nil
		}
		if r.Out != nil {
			r.Out.Step("Syncing %d changed path(s)...", len(paths))
		}
		if err := upload.SyncPaths(r.Cwd, r.Proj, r.Exec, paths, syncOpts); err != nil {
			return err
		}
		return r.recordSynced()
	}

	if r.Out != nil {
		r.Out.Success("Watching for changes. Press Ctrl+C to stop.")
	}

	for {
		select {
		case <-ctx.Done():
			return flush()
		case event, ok := <-watcher.Events:
			if !ok {
				return flush()
			}
			if isIgnorableWatchEvent(event) {
				continue
			}
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					_ = addRepoWatches(watcher, event.Name)
				}
			}
			paths := watchPathsForEvent(r.Cwd, event)
			for _, rel := range paths {
				pendingMu.Lock()
				pending[rel] = struct{}{}
				pendingMu.Unlock()
			}
			if len(paths) > 0 {
				db.Notify()
			}
		case <-db.trigger:
			if err := flush(); err != nil {
				return err
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return flush()
			}
			return err
		}
	}
}

func isIgnorableWatchEvent(event fsnotify.Event) bool {
	return event.Op == fsnotify.Chmod
}

func watchPathsForEvent(root string, event fsnotify.Event) []string {
	if event.Op&fsnotify.Rename != 0 {
		var paths []string
		if rel, ok := repoRelativePath(root, event.Name); ok && shouldWatchRel(root, rel) {
			paths = append(paths, rel)
		}
		return paths
	}
	if rel, ok := repoRelativePath(root, event.Name); ok && shouldWatchRel(root, rel) {
		return []string{rel}
	}
	return nil
}

func addRepoWatches(watcher *fsnotify.Watcher, root string) error {
	ignorePatterns := loadWatchIgnore(root)
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return watcher.Add(path)
		}
		rel = filepath.ToSlash(rel)
		if shouldIgnoreRepo(rel, ignorePatterns, true) {
			return filepath.SkipDir
		}
		return watcher.Add(path)
	})
}

func loadWatchIgnore(root string) []string {
	return upload.LoadOutpostIgnorePatterns(root)
}

func shouldWatchPath(root, path string) bool {
	return len(watchPathsForEvent(root, fsnotify.Event{Name: path})) > 0
}

func shouldWatchRel(root, rel string) bool {
	if rel == "" {
		return false
	}
	if !upload.IsSyncableRepoPath(root, rel) {
		return false
	}
	local := filepath.Join(root, rel)
	info, err := os.Stat(local)
	if err != nil {
		return true
	}
	return !info.IsDir()
}

func repoRelativePath(root, path string) (string, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || strings.HasPrefix(rel, "..") {
		return "", false
	}
	return rel, true
}

// WatchContext returns a context cancelled on SIGINT/SIGTERM.
func WatchContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
			cancel()
		case <-ctx.Done():
		}
		signal.Stop(sigCh)
	}()
	return ctx, cancel
}

// shouldIgnoreRepo mirrors upload ignore rules for watch setup.
func shouldIgnoreRepo(rel string, patterns []string, isDir bool) bool {
	return upload.ShouldIgnoreRel(rel, patterns, isDir)
}
