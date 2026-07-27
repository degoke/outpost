# Outpost

Outpost turns a remote Linux host into a shared development environment you control from your local terminal. Run Docker Compose stacks, Kubernetes clusters, and lightweight Linux machines on the server — without installing Docker, `kubectl`, or a local VM stack on your laptop.

Use an existing Linux server or let Outpost provision one on AWS. Share the host with teammates through invitation codes; collaborators get runtime access without your cloud credentials.

## What you get

- **Remote Docker and Compose** — develop against containers on a shared host, not your laptop.
- **Kubernetes with kind** — create named clusters and run `kubectl` remotely.
- **Linux machines with Incus** — system containers by default; full VMs when the host supports KVM.
- **Local port forwarding** — reach remote services at `http://127.0.0.1:8080` from your machine.
- **Team sharing** — invite collaborators with device approval; owners keep control of the host and cloud account.

## How it works

You install the Outpost CLI locally. It connects to your host over SSH, installs missing tools on first use, syncs project files, and runs commands remotely. There is no Outpost agent running on the server.

```text
Your machine                Remote Linux host
─────────────               ─────────────────
outpost CLI        SSH  →   Docker + Compose
~/.outpost/                 kind + kubectl
.outpost/project.yaml       Incus
```

When you run `outpost compose up`, Outpost uploads your compose files (and `.env` if present) to the host and starts the stack there. When you run `outpost connect`, it forwards published ports to your localhost.

## Install

**From source** (requires Go 1.26+):

```bash
go install github.com/goke/outpost/cmd/outpost@latest
```

**From a release** — download the binary for your platform from [GitHub Releases](https://github.com/goke/outpost/releases).

## Requirements

| | |
|---|---|
| **Your machine** | Outpost CLI, SSH client, network access to the host |
| **Remote host** | Linux with SSH; `sudo` for first-time setup |
| **Supported distros** | Debian/Ubuntu, Amazon Linux, RHEL, CentOS, Rocky, and similar |
| **AWS (optional)** | Configured AWS CLI profile with EC2 permissions |

## Getting started

### Use an existing server

```bash
# 1. Register the host (verifies SSH and bootstraps Docker)
# Password-only VPS (default when no --identity-file is given):
outpost host add personal --hostname 203.0.113.10 --user ubuntu --auth password

# Or with a dedicated key file:
outpost host add personal --hostname 203.0.113.10 --user ubuntu --auth key --identity-file ~/.ssh/vps_key

# 2. Re-verify later if needed
outpost host verify

# 3. Initialize your project (in a repo with docker-compose.yml)
outpost init

# 4. Start your stack
outpost compose up -d

# 5. Forward ports to your machine
outpost connect
```

Your services are now available on localhost — for example `http://127.0.0.1:8080` if that port is published in compose.

### Provision a host on AWS

```bash
outpost provider login aws --profile my-profile --region eu-west-1
outpost host create personal --provider aws --region eu-west-1
outpost host verify

outpost init
outpost compose up -d
outpost connect
```

Outpost creates the EC2 instance, configures SSH, installs Docker, and registers the host. You can start, stop, resize, or destroy it with `outpost host` commands.

## Day-to-day usage

### Hosts

```bash
outpost host list                    # list registered hosts
outpost host use personal            # switch active host
outpost host verify                  # check connection and dependencies
outpost host capabilities            # see what the host supports (e.g. VMs)
```

Use `--host NAME` on any command to target a specific host without changing the active one.

### Projects and Compose

In each repository, run `outpost init` once. It detects `docker-compose.yml` or `compose.yaml` and writes `.outpost/project.yaml` with a stable project name.

```bash
outpost init --name my-api           # set a stable name (defaults to repo folder name)
outpost init --write-gitignore       # add .outpost/ to .gitignore

outpost compose up -d
outpost compose ps
outpost compose logs -f
outpost compose exec api sh
outpost compose down

outpost docker ps
outpost docker logs my-container
```

`compose up`, `build`, and `pull` sync your compose files to the host before running. Commit `.outpost/project.yaml` to git so teammates use the same remote project. Keep secrets in `.env` and out of version control — Outpost syncs `.env` to the host when it exists locally.

### Moving compose volumes between hosts

Named Docker volumes (for example Postgres data) stay on the host they were created on. Outpost can archive them locally and restore them on another host.

```bash
# On the old host: save volumes to ~/.outpost/archives/{project}/
outpost compose volumes export

# On the new host: restore from local archives
outpost compose volumes import

# Check status
outpost compose volumes list
```

When you run `outpost compose up`, Outpost automatically offers to import missing or empty volumes that have local archives. Use `--yes` to skip the prompt.

To move a project:

```bash
outpost host use old-host
outpost compose volumes export

# point the project at the new host in .outpost/project.yaml, then:
outpost host use new-host
outpost compose volumes import
outpost compose up -d
```

### Port forwarding

```bash
outpost connect                      # forward all published compose ports
outpost connect --service api        # one service only
outpost connect --port 9090:80       # custom mapping
outpost connect --status             # show active sessions
outpost connect --down               # stop forwarding
```

**Port already in use?** Stop the local process on that port, or override with `--local-port 18080` or `--port 9090:80`.

## Sharing with your team

The host owner creates invitations; teammates join with a code and wait for approval.

```bash
# Owner
outpost invite create
outpost invite list
outpost invite approve DEVICE_ID
outpost invite revoke DEVICE_ID

# Teammate
outpost invite join CODE --hostname 203.0.113.10 --user ubuntu --label my-laptop
```

Members can run workloads (`docker`, `compose`, `connect`, `kubectl`, etc.) but cannot create or destroy hosts, manage invitations, or use cloud provider commands. Destructive operations warn when other teammates may be affected.

| Members can | Members cannot |
|-------------|----------------|
| `docker`, `compose`, `connect` | Manage hosts or invitations |
| `status`, `top`, `capacity`, `disk`, `prune` | `init`, `host create/destroy`, `provider login` |
| `cluster list`, `kubectl` | `cluster create/delete` |
| `machine shell`, `machine exec` | `machine create/delete` |

## AWS host management

```bash
outpost host create personal --provider aws --region eu-west-1
outpost host start personal
outpost host stop personal
outpost host restart personal
outpost host resize personal --instance-type t3.large
outpost host remove personal           # remove from local config only
outpost host destroy personal          # terminate the EC2 instance
```

`host remove` only forgets the host in your local config — the server keeps running. `host destroy` terminates the cloud instance.

## Kubernetes

Create and use kind clusters on the host. No local `kubectl` required.

```bash
outpost cluster create dev
outpost cluster create staging --workers 2
outpost cluster list
outpost cluster status dev
outpost kubectl --cluster dev get nodes
outpost kubectl --cluster dev apply -f ./manifest.yaml
outpost cluster delete dev
```

Local manifest files are uploaded automatically when you apply them.

## Linux machines

System containers are lightweight and work on most hosts, including standard EC2 instances:

```bash
outpost machine create ubuntu-dev --image ubuntu:24.04
outpost machine shell ubuntu-dev
outpost machine exec ubuntu-dev -- uname -a
outpost machine stop ubuntu-dev
outpost machine snapshot create ubuntu-dev
outpost machine delete ubuntu-dev
```

**Virtual machines** need KVM. They work on bare-metal servers, metal EC2 instance types, or hosts with [nested virtualization](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/nested-virtualization.html). Standard `t3.*` instances do not support VMs — use system containers instead.

```bash
outpost host capabilities
outpost machine create vm-dev --image ubuntu:24.04 --virtual-machine --cpu 2 --memory 4GiB --disk 20GiB
```

## Monitoring and cleanup

```bash
outpost status          # host health and workload summary
outpost top             # live container CPU and memory
outpost top --watch
outpost capacity        # free resources and recommendations
outpost disk            # disk usage and reclaimable space

outpost prune --dry-run # preview cleanup
outpost prune           # remove stopped containers, unused images, build cache
outpost prune volumes   # explicit: unused named volumes
```

## Configuration

Outpost stores two kinds of configuration:

| Location | Purpose |
|----------|---------|
| `~/.outpost/config.yaml` | Registered hosts, active host, AWS defaults |
| `.outpost/project.yaml` | Per-repo project name, host, and compose files |

Example project config (created by `outpost init`):

```yaml
name: my-api
host: personal
remote_dir: /var/lib/outpost/projects/my-api
compose_files:
  - docker-compose.yml
```

Use the same project name across your team so everyone targets the same remote stack.

## Command-line options

These flags work on every command:

| Flag | Description |
|------|-------------|
| `--host NAME` | Use a specific host instead of the active one |
| `--json` | JSON output |
| `--debug` | Verbose logging |
| `--yes` | Skip confirmation prompts |

## Troubleshooting

| Problem | What to try |
|---------|-------------|
| SSH connection fails | Test with `ssh user@host`. Check hostname, user, port, and key. Pass `--identity-file` to `host add` if needed. |
| Bootstrap fails | Ensure your user has `sudo` on the host. On unsupported distros, install Docker manually, then run `outpost host verify`. |
| Port forwarding conflict | Run `outpost connect --status`. Use `--local-port` or `--port` to pick a different local port. |
| Member access denied | Owner runs `outpost invite list` and approves the device. |
| Not enough resources | Run `outpost capacity` before creating stacks, clusters, or machines. |
| Start over locally | Run `outpost reset` to clear `~/.outpost` (hosts, keys, sessions). Remote servers and repo project files are kept. |
