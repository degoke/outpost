---
title: Migration
slug: guides/migration
section: guides
order: 2
---

# Migration

## Full project migration

Use [migrate](../commands/migrate) to move an entire project environment between hosts:

```bash
outpost migrate --from old-host --to new-host
outpost migrate --from old-host --to new-host --dry-run
```

This exports and restores Docker containers, volumes, Kubernetes state, optional Incus machine metadata, and updates `.outpost/project.yaml` to point at the new host.

Skip parts you do not need:

```bash
outpost migrate --from old-host --to new-host --skip-volumes --skip-cluster
```

## Granular volume migration

For moving only named Compose volumes:

```bash
# On the old host
outpost compose volumes export

# On the new host (after pointing project at new host)
outpost compose volumes import
outpost compose volumes import --force   # overwrite existing volumes
outpost compose volumes list
```

`outpost compose up` can automatically offer to import missing volumes from local archives.

## When to use which

| Scenario | Command |
|----------|---------|
| Move entire project to new server | `outpost migrate` |
| Copy Postgres/Redis data only | `compose volumes export/import` |
| Fresh start on new host | `init` on new host, skip migration |
