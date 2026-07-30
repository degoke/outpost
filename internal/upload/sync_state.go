package upload

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/degoke/outpost/internal/config"
)

// SyncScope identifies which file set a sync state entry tracks.
type SyncScope string

const (
	SyncScopeRepo    SyncScope = "repo"
	SyncScopeProject SyncScope = "project"
)

type syncStateRecord struct {
	Host        string    `json:"host"`
	Project     string    `json:"project"`
	Cwd         string    `json:"cwd"`
	Scope       SyncScope `json:"scope"`
	Fingerprint string    `json:"fingerprint"`
	SyncedAt    time.Time `json:"synced_at"`
}

func syncStateDir() (string, error) {
	dir, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sync-state"), nil
}

func syncStatePath(host, project string, scope SyncScope) (string, error) {
	dir, err := syncStateDir()
	if err != nil {
		return "", err
	}
	safeProject := config.SanitizeProjectName(project)
	return filepath.Join(dir, fmt.Sprintf("%s_%s_%s.json", host, safeProject, scope)), nil
}

func computeFingerprint(cwd string, paths []string) (string, error) {
	sorted := append([]string{}, paths...)
	sort.Strings(sorted)
	h := sha256.New()
	for _, rel := range sorted {
		local := filepath.Join(cwd, rel)
		info, err := os.Stat(local)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", err
		}
		if info.IsDir() {
			continue
		}
		mod := info.ModTime().UTC().UnixNano()
		fmt.Fprintf(h, "%s\n%d\n%d\n", rel, info.Size(), mod)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// RepoFingerprint returns a hash of local repository file metadata.
func RepoFingerprint(cwd string) (string, error) {
	paths, err := collectRepoPaths(cwd)
	if err != nil {
		return "", err
	}
	return computeFingerprint(cwd, paths)
}

// ProjectFingerprint returns a hash of compose sync path metadata.
func ProjectFingerprint(cwd string, proj *config.Project) (string, error) {
	paths, err := collectSyncPaths(cwd, proj)
	if err != nil {
		return "", err
	}
	return computeFingerprint(cwd, paths)
}

// NeedsRepoSync reports whether the local repository changed since the last recorded sync.
func NeedsRepoSync(cwd, host, project string) (bool, error) {
	return needsSync(cwd, host, project, SyncScopeRepo, func() (string, error) {
		return RepoFingerprint(cwd)
	})
}

// NeedsProjectSync reports whether compose sync inputs changed since the last recorded sync.
func NeedsProjectSync(cwd string, proj *config.Project, host string) (bool, error) {
	return needsSync(cwd, host, proj.Name, SyncScopeProject, func() (string, error) {
		return ProjectFingerprint(cwd, proj)
	})
}

func needsSync(cwd, host, project string, scope SyncScope, fingerprint func() (string, error)) (bool, error) {
	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return true, err
	}
	fp, err := fingerprint()
	if err != nil {
		return true, err
	}
	record, err := loadSyncState(host, project, scope)
	if err != nil {
		return true, nil
	}
	if record == nil {
		return true, nil
	}
	if record.Cwd != absCwd || record.Fingerprint != fp {
		return true, nil
	}
	return false, nil
}

// MarkRepoSynced records a successful repository sync.
func MarkRepoSynced(cwd, host, project string) error {
	return markSynced(cwd, host, project, SyncScopeRepo, func() (string, error) {
		return RepoFingerprint(cwd)
	})
}

// MarkProjectSynced records a successful compose project sync.
func MarkProjectSynced(cwd string, proj *config.Project, host string) error {
	return markSynced(cwd, host, proj.Name, SyncScopeProject, func() (string, error) {
		return ProjectFingerprint(cwd, proj)
	})
}

func markSynced(cwd, host, project string, scope SyncScope, fingerprint func() (string, error)) error {
	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return err
	}
	fp, err := fingerprint()
	if err != nil {
		return err
	}
	record := syncStateRecord{
		Host:        host,
		Project:     project,
		Cwd:         absCwd,
		Scope:       scope,
		Fingerprint: fp,
		SyncedAt:    time.Now().UTC(),
	}
	dir, err := syncStateDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	path, err := syncStatePath(host, project, scope)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func loadSyncState(host, project string, scope SyncScope) (*syncStateRecord, error) {
	path, err := syncStatePath(host, project, scope)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var record syncStateRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, err
	}
	if record.Host != host || record.Project != project || record.Scope != scope {
		return nil, nil
	}
	return &record, nil
}
