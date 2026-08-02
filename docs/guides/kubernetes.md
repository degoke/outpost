---
title: Kubernetes
slug: guides/kubernetes
section: guides
order: 3
---

# Kubernetes

Outpost runs one project-scoped Kubernetes cluster inside the managed development container using **kind** (default) or **k3d**.

## Setup

```bash
outpost init
outpost cluster up
outpost cluster up --driver k3d
outpost cluster status
```

The driver choice is saved to `.outpost/project.yaml`.

## Local development against the remote cluster

```bash
outpost open
outpost cluster env -- kubectl get pods
outpost cluster env -- make deploy
```

`outpost open` forwards the Kubernetes API and writes `.outpost/kubeconfig`. Your local `kubectl` and app tooling can use that file without modifying `~/.kube/config`.

## Teardown

```bash
outpost cluster down
```

## Permissions

- **Owner**: `cluster up`, `cluster down`
- **Members**: `cluster status` only; `cluster env` is owner-only because it executes arbitrary local Kubernetes commands against the project tunnel.

See [cluster command reference](../commands/cluster).
