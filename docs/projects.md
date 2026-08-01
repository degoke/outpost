---
title: Projects
slug: projects
section: overview
order: 3
---

# Projects

## The `.outpost/` folder

`outpost init` creates a `.outpost/` directory in your repository. This is **local project metadata** — it is never uploaded to the remote host.

| File | Purpose |
|------|---------|
| `project.yaml` | Project name, host override, environment, AI agent, Compose, Kubernetes, machine settings |
| `.outpostignore` | Patterns excluded from mirror sync (like `.gitignore`) |

Commit `.outpost/` so teammates share the same project name and remote path, or use `outpost init --write-gitignore` to keep it local.

```text
my-repo/
├── .outpost/
│   ├── project.yaml
│   └── .outpostignore
├── docker-compose.yml
└── src/
```

Global CLI state lives in `~/.outpost/` (hosts, SSH keys, port-forward sessions) — separate from repo metadata.

## Managed development environment

Each project gets one managed Docker container on the host when `environment.enabled` is true (default).

- Source files sync with rsync (SFTP fallback)
- `.devcontainer/devcontainer.json` is honored for image, workspace, env, ports, mounts, and builds
- Go, Node, and Python toolchains can be auto-detected and installed in auto-built images
- `outpost shell` keeps the workspace synchronized while you work
- `outpost ai` runs a remote AI agent and pulls changes back when you exit

Set `environment.enabled: false` in `project.yaml` to run directly on the host without a managed container.

Optional AI agent default:

```yaml
ai:
  command: opencode
```

## Sync exclusions

Edit `.outpost/.outpostignore`:

```gitignore
node_modules/
.venv/
dist/
*.log
```

Built-in excludes always apply: `.git/`, `.outpost/`, `.DS_Store`.

## Common commands

```bash
outpost init --name my-api
outpost init --no-compose          # script-only or Dockerfile-only repos
outpost init --no-shell            # metadata only (CI)

outpost shell
outpost ai
outpost run -- npm test
outpost compose up
outpost compose logs -f
outpost compose down
outpost status
outpost cleanup
```
