---
title: Getting started
slug: getting-started
section: overview
order: 2
---

# Getting started

## Install the CLI

**Install script** (macOS and Linux):

```bash
curl -fsSL https://raw.githubusercontent.com/degoke/outpost/main/scripts/install.sh | bash
```

Pin a version:

```bash
curl -fsSL https://raw.githubusercontent.com/degoke/outpost/main/scripts/install.sh | OUTPOST_VERSION=v0.1.0 bash
```

**From source** (Go 1.26+):

```bash
go install github.com/degoke/outpost/cmd/outpost@latest
```

Verify and enable shell completion:

```bash
outpost --help
outpost completion zsh > "${fpath[1]}/_outpost"
```

## Use an existing server

```bash
# Register the host (verifies SSH and bootstraps Docker)
outpost host add personal --hostname 203.0.113.10 --user ubuntu --auth password

# Or with an SSH key:
outpost host add personal --hostname 203.0.113.10 --user ubuntu --auth key --identity-file ~/.ssh/vps_key

outpost host verify

# In your project repository:
outpost init
outpost shell          # sync, managed container, remote shell
outpost ai             # sync, remote AI agent, pull changes back on exit
outpost compose up     # when you use Compose
outpost open           # forward ports to localhost
outpost close          # stop forwarding
```

Compose is **optional**. Projects can use a managed dev container, a Dockerfile app (`outpost app`), or host-only execution.

For CI and scripts, skip the interactive shell:

```bash
outpost init --no-shell
```

## Provision a host on AWS

```bash
outpost provider login aws --profile my-profile --region eu-west-1
outpost host create personal --provider aws --region eu-west-1
outpost host verify

outpost init
outpost compose up
outpost open
```

## Requirements

| | |
|---|---|
| **Your machine** | Outpost CLI, SSH client, network access to the host |
| **Remote host** | Linux with SSH; `sudo` for first-time setup |
| **Supported distros** | Debian/Ubuntu, Amazon Linux, RHEL, CentOS, Rocky, and similar |
| **AWS (optional)** | Configured AWS CLI profile with EC2 permissions |
