---
title: shell
slug: commands/shell
section: commands
order: 2
---

# outpost shell

Open an interactive shell in the project's managed development environment.

## Usage

```bash
outpost shell
```

## What happens

1. Syncs the repository to the remote host (if changed)
2. Ensures the managed development container is running
3. Opens an interactive bash session with toolchain and venv activation when applicable
4. Keeps the workspace synchronized in the background while the shell is open

## Notes

- **Owner only** on shared hosts.
- Type `exit` to return to your local terminal.
- Requires a project initialized with `outpost init`.

## Related

- [run](run) · [ai](ai) · [init](init) · [projects](../projects)
