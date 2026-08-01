---
title: AWS hosts
slug: guides/aws-hosts
section: guides
order: 4
---

# AWS hosts

Provision and manage EC2 instances as Outpost hosts.

## Provision

```bash
outpost provider login aws --profile my-profile --region eu-west-1
outpost host create dev --provider aws --region eu-west-1
outpost host verify
```

Outpost creates an EC2 instance (20 GiB minimum gp3 root volume), configures SSH, installs Docker, and registers the host.

## Lifecycle

```bash
outpost host stop dev
outpost host start dev
outpost host restart dev
outpost host resize dev --instance-type t3.large
outpost host update-ssh-access dev
```

`host update-ssh-access` refreshes the security group SSH ingress rule for your current public IP.

## Remove vs destroy

| Command | Effect |
|---------|--------|
| `outpost host remove dev` | Remove from local `~/.outpost` config only |
| `outpost host destroy dev` | Terminate the EC2 instance |
| `outpost host destroy dev --delete-volumes` | Also delete attached EBS volumes |

## Flags

`host create` supports `--ssh-cidr` and `--no-cleanup` for advanced provisioning scenarios.

See [host command reference](../commands/host).
