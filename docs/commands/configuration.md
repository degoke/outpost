---
title: Configuration
slug: commands/configuration
section: commands
order: 16
---

# Configuration

## Global config (`~/.outpost/`)

| Path | Purpose |
|------|---------|
| `config.yaml` | Registered hosts, active host, settings |
| `hosts/` | Per-host SSH keys and metadata |
| `sessions/` | Port-forward worker state |
| `archives/` | Exported Compose volume archives |

## Project config (`.outpost/project.yaml`)

Example fields:

```yaml
name: my-api
host: personal
remote_dir: /var/lib/outpost/projects/my-api
environment:
  enabled: true
  image: ubuntu:24.04
compose:
  files:
    - docker-compose.yml
kubernetes:
  driver: kind
machine:
  image: ubuntu:24.04
toolchain:
  auto: true
cleanup:
  containers: true
```

## Reset local state

```bash
outpost reset    # clears ~/.outpost (does not affect remote servers or repo .outpost/)
```

## Shell completion

```bash
outpost completion bash
outpost completion zsh
outpost completion fish
```

## Related

- [projects](../projects) · [init](init)
