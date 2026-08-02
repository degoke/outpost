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

Members can inspect the host but cannot run arbitrary workloads or manage infrastructure/project initialization.

Approved member keys use a server-side forced read-only command with SSH forwarding, agent, X11, and TTY access disabled. Re-run `outpost host verify` as the owner after upgrading an existing host so the restriction wrapper is installed before approving new devices.

| Members can | Members cannot |
|-------------|----------------|
| read-only `docker`, `compose ps`, `compose logs` | `compose up/down/exec/build/pull`, `docker run/exec/cp`, `init`, `shell`, `run`, `open`, `close`, `migrate`, `cleanup`, `app` |
| `status`, `top`, `capacity`, `disk`, read-only `docker`/`compose ps`/`compose logs` | `prune`, `host create/destroy/add`, `provider login`, invitation management |
| `cluster status` | `cluster env`, `cluster up`, `cluster down` |
| `machine status`, `machine snapshot list` | `machine shell/exec/copy/connect`, `machine up`, `machine down`, snapshot create/delete |
| `host verify`, `list`, `use` | Most other `host` subcommands |
| `invite join`, `reset` | `invite create`, `approve`, `revoke` |

Destructive operations warn when other teammates may be affected.
