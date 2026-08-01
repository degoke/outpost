---
title: Team sharing
slug: guides/sharing
section: guides
order: 1
---

# Team sharing

The host owner creates invitations; teammates join with a code and wait for device approval.

## Owner workflow

```bash
outpost invite create --ttl 72h
outpost invite list
outpost invite approve DEVICE_ID
outpost invite revoke DEVICE_ID
```

## Teammate workflow

```bash
outpost invite join CODE --hostname 203.0.113.10 --user ubuntu --label my-laptop
# wait for owner approval, then:
outpost host use shared-host
outpost compose up
outpost status
```

## Member permissions

Members can run workloads and inspect the host but cannot manage infrastructure or project initialization.

| Members can | Members cannot |
|-------------|----------------|
| `docker`, `compose` | `init`, `shell`, `run`, `open`, `close`, `migrate`, `cleanup`, `app` |
| `status`, `top`, `capacity`, `disk`, `prune` (not clusters/machines) | `host create/destroy/add`, `provider login`, invitation management |
| `cluster status`, `cluster env` | `cluster up`, `cluster down` |
| `machine status`, `shell`, `exec`, `copy`, `connect` | `machine up`, `machine down` |
| `machine snapshot create`, `list` | `machine snapshot delete` |
| `host verify`, `list`, `use` | Most other `host` subcommands |
| `invite join`, `reset` | `invite create`, `approve`, `revoke` |

Destructive operations warn when other teammates may be affected.
