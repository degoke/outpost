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
outpost run -- npm test
outpost run -- make build
outpost run -- python script.py
```

## Notes

- **Owner only** on shared hosts.
- Auto-installs detected toolchains (Go, make, etc.) when `toolchain.auto` is enabled.
- For Python projects, creates and uses a remote `.venv` automatically.
- Exit code is propagated to your local shell.

## Related

- [shell](shell) · [ai](ai) · [app](app)
