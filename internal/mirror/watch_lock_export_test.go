package mirror

// AcquireWatchLockForTest exposes watch lock acquisition for tests.
func AcquireWatchLockForTest(host, project, cwd string) (func(), error) {
	return acquireWatchLock(host, project, cwd)
}
