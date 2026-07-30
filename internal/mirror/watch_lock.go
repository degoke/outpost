package mirror

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/degoke/outpost/internal/config"
)

type watchLock struct {
	Host      string    `json:"host"`
	Project   string    `json:"project"`
	Cwd       string    `json:"cwd"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
}

func watchLockDir() (string, error) {
	dir, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "mirror-watch"), nil
}

func watchLockPath(host, project string) (string, error) {
	dir, err := watchLockDir()
	if err != nil {
		return "", err
	}
	safeProject := config.SanitizeProjectName(project)
	return filepath.Join(dir, fmt.Sprintf("%s_%s.json", host, safeProject)), nil
}

func acquireWatchLock(host, project, cwd string) (func(), error) {
	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return nil, err
	}
	dir, err := watchLockDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	path, err := watchLockPath(host, project)
	if err != nil {
		return nil, err
	}
	lock := watchLock{
		Host:      host,
		Project:   project,
		Cwd:       absCwd,
		PID:       os.Getpid(),
		StartedAt: time.Now().UTC(),
	}
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return nil, err
	}
	release := func() {
		_ = os.Remove(path)
	}
	return release, nil
}

// WatchActive reports whether mirror watch is running for the same host, project, and working directory.
func WatchActive(host, project, cwd string) bool {
	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return false
	}
	path, err := watchLockPath(host, project)
	if err != nil {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var lock watchLock
	if err := json.Unmarshal(data, &lock); err != nil {
		return false
	}
	if lock.Host != host || lock.Project != project || lock.Cwd != absCwd {
		return false
	}
	return processAlive(lock.PID)
}
