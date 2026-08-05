---
title: session
slug: commands/session
section: commands
order: 5
---

# outpost session

Inspect and reconnect to persistent `outpost run` sessions.

## Usage

```bash
outpost session list
outpost session status NAME
outpost session attach NAME
outpost session logs NAME
outpost session logs NAME -f
outpost session kill NAME
```

## Notes

- Run sessions are backed by remote tmux and survive SSH disconnects.
- `session list` shows running sessions and recently finished sessions recorded in local metadata.
- Finished session metadata is pruned automatically after 7 days.
- Logs are stored on the remote host under `.outpost/sessions/`.
- Detaching from a running session (`Ctrl+b d`) exits with code 130 so scripts can tell the job is still running.
- **Owner only** on shared hosts.

## Related

- [run](run) · [shell](shell)
