---
title: run
slug: commands/run
section: commands
order: 4
---

# outpost run

Run a command in the remote project environment.

## Usage

```bash
outpost run -- python train.py
outpost run --name batch1 --detach -- python generate.py
outpost run --attach batch1
outpost run --foreground -- npm test
```

## Flags

| Flag | Description |
|------|-------------|
| `--detach`, `-d` | Start the command and return immediately (do not attach) |
| `--name`, `-n` | Session name (default: timestamped `run-YYYYMMDD-HHMMSS`) |
| `--attach`, `-a` | Attach to an existing session |
| `--foreground` | Legacy mode: run attached directly over SSH (stops on disconnect) |

## Behavior

By default, `outpost run` starts a **persistent tmux session** on the remote host, attaches your terminal, and streams output. If SSH drops or you close your laptop, the command keeps running.

Reconnect later:

```bash
outpost session list
outpost session attach batch1
outpost session status batch1
outpost session logs batch1 -f
```

Use `--detach` when you want to fire-and-forget:

```bash
outpost run --name batch1 --detach -- python generate.py
```

## Notes

- **Owner only** on shared hosts.
- Auto-installs detected toolchains (Go, make, etc.) when `toolchain.auto` is enabled.
- For Python projects, creates and uses a remote `.venv` automatically.
- Exit code is propagated to your local shell when the session finishes while attached.
- Detaching from a still-running session (`Ctrl+b d`) exits with code **130** so scripts know the job did not finish.
- If a session ends without writing an exit marker, the CLI exits with code **1**.
- Use `--foreground` for quick commands or CI where tmux overhead is unnecessary.

## Related

- [session](session) · [shell](shell) · [ai](ai) · [app](app)
