---
title: host
slug: commands/host
section: commands
order: 13
---

# outpost host

Register, inspect, and manage remote hosts.

## Usage

```bash
outpost host add NAME --hostname HOST --user USER --auth password
outpost host add NAME --hostname HOST --user USER --auth key --identity-file ~/.ssh/key
outpost host add NAME --hostname HOST --skip-bootstrap
outpost host create NAME --provider aws --region eu-west-1
outpost host list
outpost host use NAME
outpost host verify
outpost host capabilities
outpost host start NAME
outpost host stop NAME
outpost host restart NAME
outpost host resize NAME --instance-type t3.large
outpost host update-ssh-access NAME
outpost host remove NAME
outpost host destroy NAME --delete-volumes
```

Top-level alias: `outpost use NAME` equals `outpost host use NAME`.

## Notes

- Host management is **owner only** except `list`, `verify`, and `use` (members).
- `host add` verifies SSH and bootstraps Docker unless `--skip-bootstrap` is set.
- `host create` provisions EC2 via AWS credentials from `outpost provider login`.

## Related

- [guides/aws-hosts](../guides/aws-hosts) · [getting-started](../getting-started)
