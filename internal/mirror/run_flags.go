package mirror

import "fmt"

// RunCLIFlags holds flags parsed from an outpost run invocation before COMMAND.
type RunCLIFlags struct {
	Detach     bool
	Foreground bool
	Attach     string
	Name       string
}

// ParseRunCLIArgs splits outpost run flags from the command to execute.
// Parsing stops at the first bare "--" or the first non-flag argument.
func ParseRunCLIArgs(args []string) (RunCLIFlags, []string, error) {
	var flags RunCLIFlags
	var cmd []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			cmd = append(cmd, args[i+1:]...)
			break
		}
		switch arg {
		case "-d", "--detach":
			flags.Detach = true
		case "--foreground":
			flags.Foreground = true
		case "-n", "--name":
			if i+1 >= len(args) {
				return flags, nil, fmt.Errorf("flag %s requires a session name", arg)
			}
			flags.Name = args[i+1]
			i++
		case "-a", "--attach":
			if i+1 >= len(args) {
				return flags, nil, fmt.Errorf("flag %s requires a session name", arg)
			}
			flags.Attach = args[i+1]
			i++
		default:
			if stringsHasFlagPrefix(arg) {
				return flags, nil, fmt.Errorf("unknown flag %q", arg)
			}
			cmd = append(cmd, args[i:]...)
			i = len(args)
		}
	}
	if len(flags.Attach) > 0 && (flags.Detach || flags.Foreground || len(flags.Name) > 0) {
		return flags, nil, fmt.Errorf("--attach cannot be combined with --detach, --foreground, or --name")
	}
	if flags.Detach && flags.Foreground {
		return flags, nil, fmt.Errorf("--detach and --foreground are mutually exclusive")
	}
	return flags, cmd, nil
}

func stringsHasFlagPrefix(arg string) bool {
	return len(arg) > 1 && arg[0] == '-'
}
