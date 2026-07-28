package mirror

import (
	"context"
	"fmt"
	"strings"

	"github.com/degoke/outpost/internal/transport"
)

const tmuxInstallScript = `
set -e
if command -v tmux >/dev/null 2>&1; then
  exit 0
fi
need_sudo=""
if [ "$(id -u)" -ne 0 ]; then need_sudo="sudo"; fi
if command -v apt-get >/dev/null 2>&1; then
  $need_sudo apt-get update -qq && $need_sudo apt-get install -y -qq tmux
elif command -v yum >/dev/null 2>&1; then
  $need_sudo yum install -y -q tmux
else
  echo "OUTPOST_ERROR: tmux not installed and no supported package manager found"
  exit 1
fi
`

func EnsureTmux(ctx context.Context, exec transport.Executor) error {
	code, err := exec.Run(ctx, "command -v tmux >/dev/null 2>&1", transport.RunOpts{})
	if err != nil {
		return err
	}
	if code == 0 {
		return nil
	}
	var stderr strings.Builder
	code, err = exec.Run(ctx, tmuxInstallScript, transport.RunOpts{Stderr: &stderr})
	if err != nil {
		return err
	}
	if code != 0 {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = "unknown error"
		}
		return fmt.Errorf("tmux install failed (exit %d): %s", code, msg)
	}
	return nil
}
