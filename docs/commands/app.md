---
title: app
slug: commands/app
section: commands
order: 11
---

# outpost app

Build and run a Dockerfile-based application for the project (separate from the managed dev environment used by `run`).

## Usage

```bash
outpost app build
outpost app run --detach --port 8080:8080
outpost app status
outpost app logs -f
outpost app stop
```

## Notes

- **Owner only** on shared hosts.
- Use when the repository has a `Dockerfile` but no Compose file — pair with `outpost init --no-compose`.
- `outpost run -- CMD` is for development commands inside the managed environment.

## Related

- [run](run) · [init](init)
