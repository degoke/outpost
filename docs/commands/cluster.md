---
title: cluster
slug: commands/cluster
section: commands
order: 9
---

# outpost cluster

Manage a project-scoped Kubernetes cluster (kind or k3d) inside the managed development container.

## Usage

```bash
outpost cluster up
outpost cluster up --driver k3d
outpost cluster status
outpost cluster env -- make deploy
outpost cluster down
```

## Flags

| Flag | Description |
|------|-------------|
| `--driver` | `kind` (default) or `k3d` — saved to `project.yaml` |

## Notes

- `cluster up` and `cluster down` are **owner only**.
- Members can run `cluster status`. `cluster env` is owner-only because it executes arbitrary local Kubernetes commands against the project tunnel.
- Use [open](open) to forward the Kubernetes API and application ports; kubeconfig is written to `.outpost/kubeconfig`.

## Related

- [open](open) · [guides/kubernetes](../guides/kubernetes)
