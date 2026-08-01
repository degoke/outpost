---
title: docker
slug: commands/docker
section: commands
order: 12
---

# outpost docker

Passthrough to the remote Docker CLI.

## Usage

```bash
outpost docker ps
outpost docker logs my-container
outpost docker exec -it my-container bash
```

## Notes

- Members can run `docker` on shared hosts.
- Interactive flags (`-it`) are adjusted automatically when running over SSH so the terminal is not double-allocated.

## Related

- [compose](compose) · [projects](../projects)
