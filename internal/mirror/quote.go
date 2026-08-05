package mirror

import (
	"strings"
)

// JoinCommandArgs reconstructs a shell command from arguments already parsed
// by the local CLI. Quoting each argument preserves embedded spaces, quotes,
// and shell metacharacters when the command is executed remotely.
func JoinCommandArgs(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = shellQuote(arg)
	}
	return strings.Join(quoted, " ")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n'\"\\$`!*?[]#~|&;()<>") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
