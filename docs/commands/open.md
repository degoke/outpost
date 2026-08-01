---
title: open
slug: commands/open
section: commands
order: 4
---

# outpost open

Forward and display project ports on localhost. Starts a background port-forwarding worker.

## Usage

```bash
outpost open
outpost open --port 9090:80
outpost open --port 8080 --local-port 3000
outpost open --service web
```

With no flags, Outpost discovers published ports from Compose, the managed environment, and Kubernetes (when configured). Writes `.outpost/kubeconfig` when a project cluster is active.

## Flags

| Flag | Description |
|------|-------------|
| `--port` | Forward a port mapping (`local:remote` or remote port only). Repeat for multiple mappings. When set, automatic discovery is skipped. |
| `--local-port` | Bind a single discovered or manual mapping to this local port. |
| `--service` | Limit discovery to one Compose service name. |

## Examples

```bash
# Discover all published ports
outpost open

# Forward container port 80 to localhost:9090
outpost open --port 9090:80

# Forward the only discovered service on localhost:3000
outpost open --local-port 3000

# Discover ports for one Compose service
outpost open --service api
```

## Notes

- **Owner only** on shared hosts.
- Services become available at `http://127.0.0.1:PORT` on your machine.
- Stop forwarding with [close](close).
- If a port is already in use locally, stop the conflicting process or adjust your Compose port mappings.

## Related

- [close](close) · [compose](compose) · [cluster](cluster)
