package connect

import (
	"fmt"
	"os"
	"time"
)

const WorkerEnvKey = "OUTPOST_CONNECT_WORKER"

func IsWorker() bool {
	return os.Getenv(WorkerEnvKey) == "1"
}

func WaitForSession(host, project string, timeout time.Duration) (*Session, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		sess, err := LoadActiveSession(host, project)
		if err == nil {
			return sess, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, fmt.Errorf("timed out waiting for forwarding session")
}
