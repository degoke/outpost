package docker

import "strings"

// IsDestructive reports whether docker passthrough args may affect shared resources.
func IsDestructive(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "rm", "rmi", "volume", "system", "compose", "container":
		return isDestructiveSubcommand(args)
	default:
		return false
	}
}

func isDestructiveSubcommand(args []string) bool {
	if len(args) < 2 {
		return false
	}
	switch args[0] {
	case "rm", "rmi":
		return true
	case "volume":
		return args[1] == "rm" || args[1] == "prune"
	case "system":
		return args[1] == "prune"
	case "compose":
		return args[1] == "down"
	case "container":
		return args[1] == "rm" || args[1] == "prune"
	default:
		return false
	}
}

// ActionLabel returns a short label for confirmation prompts.
func ActionLabel(args []string) string {
	if len(args) == 0 {
		return "docker"
	}
	end := len(args)
	if end > 3 {
		end = 3
	}
	return "docker " + strings.Join(args[:end], " ")
}
