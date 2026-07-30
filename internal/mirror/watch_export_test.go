package mirror

import "github.com/fsnotify/fsnotify"

// RepoRelativePathForTest exposes path normalization for tests.
func RepoRelativePathForTest(root, path string) (string, bool) {
	return repoRelativePath(root, path)
}

// ShouldWatchRelForTest exposes watch filtering for tests.
func ShouldWatchRelForTest(root, rel string) bool {
	return shouldWatchRel(root, rel)
}

// IsIgnorableWatchEventForTest exposes watch event filtering for tests.
func IsIgnorableWatchEventForTest(event fsnotify.Event) bool {
	return isIgnorableWatchEvent(event)
}
