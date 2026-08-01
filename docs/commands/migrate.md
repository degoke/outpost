---
title: migrate
slug: commands/migrate
section: commands
order: 6
---

# outpost migrate

Migrate a project's full environment from one host to another — containers, volumes, Kubernetes state, optional Incus machine, and remote `.outpost` metadata.

## Usage

```bash
outpost migrate --from old-host --to new-host
outpost migrate --from old-host --to new-host --dry-run
outpost migrate --from old-host --to new-host --skip-volumes
outpost migrate --from old-host --to new-host --skip-cluster --skip-machine --skip-compose
```

## Flags

| Flag | Description |
|------|-------------|
| `--from` | Source host name (required) |
| `--to` | Destination host name (required) |
| `--dry-run` | Show what would be migrated without making changes |
| `--skip-volumes` | Skip Docker volume export/import |
| `--skip-cluster` | Skip Kubernetes state migration |
| `--skip-machine` | Skip Incus machine migration |
| `--skip-compose` | Skip starting Compose on the destination |

## Notes

- **Owner only.**
- Updates the project's host in `.outpost/project.yaml` on success.
- For granular volume moves without a full migration, see [Migration guide](../guides/migration).

## Related

- [compose](compose) · [guides/migration](../guides/migration)
