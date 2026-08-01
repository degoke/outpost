---
title: Documentation
slug: index
section: overview
order: 1
---

# Outpost documentation

Outpost turns a remote Linux host into a shared development environment you control from your local terminal. Run Docker Compose, Kubernetes, and Linux machines on infrastructure you own — without installing Docker, `kubectl`, or a local VM stack on your laptop.

## How it works

You install the Outpost CLI locally. It connects to your host over SSH, syncs project files, and manages a per-project development container on the remote host. There is no permanent Outpost agent on the server.

```text
Your machine                Remote Linux host
─────────────               ─────────────────
outpost CLI        SSH  →   Docker + Compose + project containers
.outpost/kubeconfig         kind/k3d inside the project container
~/.outpost/ (global)        Kubernetes node containers + Incus
```

## Project-first workflow

| Step | Command | What it does |
|------|---------|--------------|
| 1 | `outpost init` | Create `.outpost/project.yaml` (metadata-first) |
| 2 | `outpost shell` | Sync repo, start managed container, open shell |
| 3 | `outpost run -- CMD` | Run a command in the project environment |
| 4 | `outpost compose up` | Start Compose services on the remote host |
| 5 | `outpost open` | Forward ports to localhost |
| 6 | `outpost close` | Stop port forwarding |

`init` writes project metadata. Sync, container creation, and the managed environment start when you run `shell`, `run`, or `compose up` — or immediately when `init` opens an interactive shell (default in a TTY).

## Global flags

Every command accepts:

| Flag | Description |
|------|-------------|
| `--host NAME` | Target a specific registered host |
| `--json` | Machine-readable JSON output |
| `--debug` | Debug logging on stderr |
| `--yes` | Skip confirmation prompts |

## Next steps

- [Getting started](getting-started) — install, connect a host, initialize a project
- [Projects](projects) — `.outpost/`, managed environments, sync rules
- [Migrate](guides/migration) — move a full project to another host
