---
title: Monitoring
slug: commands/monitoring
section: commands
order: 15
---

# Monitoring and cleanup

Inspect host health, resource usage, and reclaim disk space.

## Usage

```bash
outpost status
outpost top
outpost top --watch
outpost capacity
outpost disk

outpost prune --dry-run
outpost prune
outpost prune volumes
outpost prune clusters    # owner only
outpost prune machines    # owner only
```

## Notes

- `status`, `top`, `capacity`, `disk`, and default `prune` are available to **members**.
- `prune clusters` and `prune machines` are **owner only**.
- Project [cleanup](cleanup) is separate from host-wide `prune`.

## Related

- [cleanup](cleanup) · [status](monitoring)
