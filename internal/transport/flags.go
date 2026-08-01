package transport

import "strings"

// StripChildTTY rewrites docker-style flags so a child process does not
// allocate its own TTY when the parent SSH session already owns one.
func StripChildTTY(args []string) []string {
	if len(args) == 0 {
		return args
	}
	out := make([]string, 0, len(args))
	for _, arg := range args {
		switch {
		case arg == "-it":
			out = append(out, "-i")
		case arg == "-t":
			continue
		case strings.HasPrefix(arg, "-it"):
			out = append(out, "-i"+strings.TrimPrefix(arg, "-it"))
		default:
			out = append(out, arg)
		}
	}
	return out
}
