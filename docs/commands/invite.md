---
title: invite
slug: commands/invite
section: commands
order: 14
---

# outpost invite

Share runtime access to a host while the owner keeps cloud and invitation control.

## Usage

```bash
# Owner
outpost invite create
outpost invite create --ttl 48h
outpost invite list
outpost invite approve DEVICE_ID
outpost invite revoke DEVICE_ID

# Teammate
outpost invite join CODE --hostname HOST --user USER --label my-laptop
outpost invite join CODE --host registered-host --label my-laptop
```

## Flags

| Flag | Description |
|------|-------------|
| `--ttl` | Invitation lifetime (default `72h`) |
| `--host` | Join via a registered host name (`invite join` only) |
| `--hostname`, `--user`, `--label` | Teammate connection details for `invite join` |

## Notes

- Invitation management is **owner only**; teammates use `invite join`.
- See [Sharing guide](../guides/sharing) for the member permission model.

## Related

- [guides/sharing](../guides/sharing)
