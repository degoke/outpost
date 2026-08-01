---
title: cleanup
slug: commands/cleanup
section: commands
order: 7
---

# outpost cleanup

Remove stale project-owned Docker artifacts, logs, and managed resources according to project cleanup settings.

## Usage

```bash
outpost cleanup
```

## Notes

- **Owner only** on shared hosts.
- Cleanup also runs automatically before other project commands when configured in `project.yaml`.

## Related

- [monitoring](monitoring) · [projects](../projects)
