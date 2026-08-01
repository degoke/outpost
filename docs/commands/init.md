---
title: init
slug: commands/init
section: commands
order: 1
---

# outpost init

Initialize Outpost for the current repository. Creates `.outpost/project.yaml` and related metadata.

## Usage

```bash
outpost init
outpost init --name my-api
outpost init --host work
outpost init --write-gitignore
outpost init --no-compose
outpost init --no-shell
```

## Flags

| Flag | Description |
|------|-------------|
| `--name` | Stable project name (defaults to repo folder name) |
| `--host` | Host override for this project |
| `--write-gitignore` | Append `.outpost/` to `.gitignore` |
| `--no-compose` | Initialize without Compose (Compose is optional) |
| `--no-shell` | Skip opening the remote shell after init |

## Notes

- **Owner only** on shared hosts.
- `init` is metadata-first: sync and the managed container start when you open a shell (`init` in a TTY by default) or run `shell`, `run`, or `compose up`.
- Compose and `.devcontainer/` are optional — not required to initialize.

## Related

- [shell](shell) · [projects](../projects) · [compose](compose)
