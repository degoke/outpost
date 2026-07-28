package mirror

import (
	"fmt"
	"strings"
	"time"

	"github.com/degoke/outpost/internal/config"
)

func sessionPrefix(proj *config.Project) string {
	return "outpost-" + proj.Name + "-"
}

func TmuxSessionName(proj *config.Project, shortName string) string {
	return sessionPrefix(proj) + shortName
}

func ShortSessionName(proj *config.Project, tmuxName string) (string, bool) {
	prefix := sessionPrefix(proj)
	if !strings.HasPrefix(tmuxName, prefix) {
		return "", false
	}
	short := strings.TrimPrefix(tmuxName, prefix)
	if short == "" {
		return "", false
	}
	return short, true
}

func DefaultSessionName() string {
	return "run-" + time.Now().UTC().Format("20060102-150405")
}

func SanitizeSessionName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("session name is required")
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "", fmt.Errorf("invalid session name %q", name)
	}
	return out, nil
}

func remoteSessionsDir(proj *config.Project) string {
	return proj.RemoteDir + "/.outpost/sessions"
}

func remoteSessionLog(proj *config.Project, shortName string) string {
	return remoteSessionsDir(proj) + "/" + shortName + ".log"
}
