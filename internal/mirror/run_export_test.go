package mirror

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// DetachedInnerCommandForTest exposes detached command builder for tests.
func DetachedInnerCommandForTest(cmd, logPath string) string {
	return detachedInnerCommand(cmd, logPath)
}

// DecodeDetachedCommandForTest reverses the base64 payload embedded in a detached command.
func DecodeDetachedCommandForTest(inner string) (string, error) {
	const marker = `printf %s `
	idx := strings.Index(inner, marker)
	if idx < 0 {
		return "", fmt.Errorf("missing base64 payload")
	}
	rest := inner[idx+len(marker):]
	end := strings.Index(rest, " | base64 -d)")
	if end < 0 {
		return "", fmt.Errorf("missing base64 trailer")
	}
	quoted := strings.Trim(rest[:end], "'")
	raw, err := base64.StdEncoding.DecodeString(quoted)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
