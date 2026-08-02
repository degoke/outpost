---
title: machine
slug: commands/machine
section: commands
order: 10
---

# outpost machine

Manage a project-scoped Linux machine with Incus — lightweight containers by default, full VMs when KVM is available.

## Usage

```bash
outpost machine up --image ubuntu:24.04
outpost machine up --cpu 2 --memory 2GiB --disk 20GiB
outpost machine up --image ubuntu:24.04 --virtual-machine
outpost machine status
outpost machine shell
outpost machine exec -- uname -a
outpost machine copy ./app project:/tmp/app
outpost machine copy project:/tmp/out.log ./out.log
outpost machine connect --port 8080:80
outpost machine snapshot create
outpost machine snapshot list
outpost machine snapshot delete NAME
outpost machine down
```

## Notes

- `machine up` and `machine down` are **owner only**.
- Members can use `machine status` and snapshot list only. Shell, exec, copy, connect, and snapshot creation are owner-only because they can mutate or expose the host runtime.
- Check `outpost host capabilities` for KVM support before creating VMs.

## Related

- [host](host) · [open](open)
