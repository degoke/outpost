---
title: compose
slug: commands/compose
section: commands
order: 8
---

# outpost compose

Run Docker Compose against the remote host for the current project.

## Usage

```bash
outpost compose up
outpost compose up -d
outpost compose down
outpost compose ps
outpost compose logs -f
outpost compose exec SERVICE bash
outpost compose build
outpost compose pull
```

## Volume migration

```bash
outpost compose volumes list
outpost compose volumes export
outpost compose volumes import
outpost compose volumes import --force
```

## Notes

- Members can run `compose` on shared hosts.
- `up` syncs project files before starting services.
- Port forwarding uses top-level [open](open), not a compose subcommand.

## Related

- [open](open) · [migrate](migrate) · [guides/migration](../guides/migration)
