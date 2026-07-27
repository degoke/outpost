package mirror

import (
	"strings"
)

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n'\"\\$`!*?[]#~|&;()<>") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
