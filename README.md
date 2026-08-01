# Outpost

<p align="center">
  <img src="./outpost.png" alt="Outpost — remote power, local control" width="640" />
</p>

Outpost turns a remote Linux host into a shared development environment you control from your local terminal. Run Docker Compose stacks, Kubernetes clusters, and lightweight Linux machines on the server — without installing Docker, `kubectl`, or a local VM stack on your laptop.

Use an existing Linux server or let Outpost provision one on AWS. Share the host with teammates through invitation codes; collaborators get runtime access without your cloud credentials.

**Full documentation:** see the [`docs/`](docs/) folder in this repository, or browse the [online docs](https://github.com/degoke/outpost#docs) (`#docs` on the project site).

## What you get

- **Remote Docker and Compose** — develop against containers on a shared host, not your laptop.
- **Project Kubernetes with kind or k3d** — run a remote cluster while your local application uses its project kubeconfig.
- **Linux machines with Incus** — system containers by default; full VMs when the host supports KVM.
- **Local port forwarding** — reach remote services at `http://127.0.0.1:8080` from your machine.
- **Remote development environments** — each project gets a managed container, with rsync-first sync and Dev Container support.
- **Team sharing** — invite collaborators with device approval; owners keep control of the host and cloud account.



## How it works

You install the Outpost CLI locally. It connects to your host over SSH, syncs project files, and manages a per-project development container on the remote host. There is no permanent Outpost agent running on the server.

```text
Your machine                Remote Linux host
─────────────               ─────────────────
outpost CLI        SSH  →   Docker + Compose + project containers
.outpost/kubeconfig         kind/k3d inside the project container
~/.outpost/ (global)        Kubernetes node containers + Incus
```

The normal workflow is `outpost init`, `outpost shell`, `outpost ai`, `outpost run`, `outpost compose up`, and `outpost open`. `init` writes project metadata first; sync, the managed container, and an interactive shell happen when you run `shell`, `ai`, `run`, `compose up`, or when `init` opens a shell in an interactive terminal (use `--no-shell` to skip).

## Install

**Install script** (macOS and Linux):

```bash
curl -fsSL https://raw.githubusercontent.com/degoke/outpost/main/scripts/install.sh | bash
```

Pin a version:

```bash
curl -fsSL https://raw.githubusercontent.com/degoke/outpost/main/scripts/install.sh | OUTPOST_VERSION=v0.1.0 bash
```

The script installs to `~/.local/bin` by default. Override with `OUTPOST_INSTALL_DIR`.

**From source** (requires Go 1.26+):

```bash
go install github.com/degoke/outpost/cmd/outpost@latest
```

**Manual download** — binaries for each platform are on [GitHub Releases](https://github.com/degoke/outpost/releases).

## Requirements


|                       |                                                               |
| --------------------- | ------------------------------------------------------------- |
| **Your machine**      | Outpost CLI, SSH client, network access to the host           |
| **Remote host**       | Linux with SSH; `sudo` for first-time setup                   |
| **Supported distros** | Debian/Ubuntu, Amazon Linux, RHEL, CentOS, Rocky, and similar |
| **AWS (optional)**    | Configured AWS CLI profile with EC2 permissions               |




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

# 3. Initialize the project (Compose and .devcontainer/ are optional)
outpost init
# Or for CI/scripts:
outpost init --no-shell

# 4. Re-enter the project shell later
outpost shell

# 5. Start services and forward ports when needed
outpost compose up
outpost open
outpost close
```

Your services are now available on localhost — for example `http://127.0.0.1:8080` if that port is published in compose.

### Provision a host on AWS

```bash
outpost provider login aws --profile my-profile --region eu-west-1
outpost host create personal --provider aws --region eu-west-1
outpost host verify

outpost init
outpost compose up
outpost open
```

Outpost creates the EC2 instance with a 20 GiB minimum gp3 root volume, configures SSH, installs Docker, and registers the host. You can start, stop, resize, or destroy it with `outpost host` commands.

## Day-to-day usage



### Hosts

```bash
outpost host list                    # list registered hosts
outpost host use personal            # switch active host
outpost host verify                  # check connection and dependencies
outpost host capabilities            # see what the host supports (e.g. VMs)
```

Use `--host NAME` on any command to target a specific host without changing the active one.

### Projects

In each repository, run `outpost init` once. It creates a `.outpost/` directory with your project configuration.

#### The `.outpost/` folder (in your repo)

When you run `outpost init`, Outpost creates a `.outpost/` directory at the root of your repository. This is **local project metadata** — it tells the CLI how to map your repo to a remote workspace. It is **never synced** to the remote host.

| File | Purpose |
|------|---------|
| `project.yaml` | Stable project name, optional host override, Compose/environment settings, Kubernetes driver, machine settings, and remote directory path |
| `.outpostignore` | Patterns for files/folders to exclude from sync (same syntax as `.gitignore`) |

**Should you commit it?** By default, yes — commit `.outpost/` so teammates use the same project name and land in the same remote directory. If you prefer per-developer settings, run `outpost init --write-gitignore` to keep `.outpost/` out of git.

```text
my-repo/
├── .outpost/
│   ├── project.yaml      # shared project config (usually committed)
│   └── .outpostignore    # sync exclusions (edit as needed)
├── docker-compose.yml
└── src/
```

This is separate from `~/.outpost/` on your machine, which stores global CLI state (registered hosts, SSH keys, port-forward sessions). See [Configuration](#configuration) below.

```bash
outpost init --name my-api           # set a stable name (defaults to repo folder name)
outpost init --write-gitignore       # keep .outpost/ local instead of committing it
outpost init --no-shell              # metadata only (CI and automation)
outpost init --no-compose            # script-only or Dockerfile-only repos

outpost shell
outpost run -- npm test
outpost compose up
outpost app build
outpost app run --detach --port 8080:8080
outpost app status
outpost app logs -f
outpost app stop
outpost status
outpost compose logs -f
outpost open
outpost compose down
outpost cleanup

outpost docker ps
outpost docker logs my-container
```

`up` syncs required files before running. Keep secrets in `.env` and out of version control — Outpost syncs `.env` to the host when it exists locally.

Create `.outpost/.outpostignore` (created automatically by `outpost init`) to exclude paths from sync. Same syntax as `.gitignore`. In git repositories, it applies **in addition to** `.gitignore`:

```gitignore
# .outpost/.outpostignore
node_modules/
.venv/
dist/
*.log
```

Built-in excludes always apply: `.git/`, `.outpost/`, `.DS_Store`.

### Remote environment

Each project gets one managed development container on the host. Source files sync with rsync (automatic SFTP fallback), and common dependency directories use persistent Docker volumes. `.devcontainer/devcontainer.json` is picked up automatically for image, workspace, environment, ports, mounts, and Dockerfile builds.

For Python projects, `outpost run` automatically creates and uses a remote `.venv`. For Go, make, and other build tools, `outpost run` auto-installs the toolchain in the environment. Set `environment.enabled: false` in `.outpost/project.yaml` to opt out of the managed container and execute directly on the host.

### Dockerfile applications

For a repository with a `Dockerfile` but no Compose file, use the project application commands:

```bash
outpost init --no-compose
outpost app build
outpost app run --detach --port 8080:8080
outpost app logs -f
outpost app stop
```

`outpost run -- COMMAND` remains for development commands inside the managed project environment. `outpost app run` runs the image built from the repository Dockerfile.

### Moving projects and volumes between hosts

**Full migration** (containers, volumes, Kubernetes state, optional Incus machine, project metadata):

```bash
outpost migrate --from old-host --to new-host
outpost migrate --from old-host --to new-host --dry-run
```

**Granular volume migration** — named Docker volumes stay on the host they were created on. Outpost can archive them locally and restore them on another host.

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
outpost compose up
```



### Port forwarding

```bash
outpost open                         # discover and forward project ports
outpost open --port 9090:80          # forward a specific mapping
outpost open --local-port 3000       # bind a single service locally
outpost open --service web           # discover ports for one Compose service
outpost close                        # stop project port forwarding
```

**Port already in use?** Stop the local process on that port, or adjust port mappings in your Compose or service configuration.

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

Members can run workloads (`docker`, `compose`, and runtime inspection commands) but cannot initialize projects, open port forwarding, migrate hosts, or manage infrastructure. Destructive operations warn when other teammates may be affected.

| Members can | Members cannot |
| ----------- | -------------- |
| `docker`, `compose` | `init`, `shell`, `run`, `open`, `close`, `migrate`, `cleanup`, `app` |
| `status`, `top`, `capacity`, `disk`, `prune` (not `prune clusters` / `prune machines`) | Manage hosts, invitations, or `provider login` |
| `cluster status`, `cluster env` | `cluster up`, `cluster down` |
| `machine status`, `shell`, `exec`, `copy`, `connect`, snapshot create/list | `machine up`, `machine down`, snapshot delete |
| `host verify`, `list`, `use`; `invite join` | Most other `host` subcommands |




## AWS host management

```bash
outpost host create personal --provider aws --region eu-west-1
outpost host stop personal            # stop EC2 instance, pause compute billing
outpost host start personal           # start again and wait for SSH
outpost host restart personal
outpost host resize personal --instance-type t3.large
outpost host update-ssh-access personal  # refresh SSH ingress for your current IP
outpost host remove personal           # remove from local config only
outpost host destroy personal          # terminate the EC2 instance
outpost host destroy personal --delete-volumes
```

`stop` pauses the instance without deleting it — you avoid EC2 compute charges while it is stopped. Attached EBS volumes (and Elastic IPs) may still bill. `start` brings the host back and waits for SSH.

`host remove` only forgets the host in your local config — the server keeps running. `host destroy` terminates the cloud instance.

## Kubernetes

Kubernetes is project-scoped. The managed project container receives the remote Docker socket, and Outpost runs either **kind** (the default) or **k3d** inside that container. The resulting Kubernetes node containers stay on the remote host.

```bash
outpost init
outpost cluster up                 # defaults to kind; saves the choice
outpost open                       # forwards app ports and the Kubernetes API
outpost cluster env -- make run    # runs locally with the project kubeconfig
```

Choose k3d when creating the project cluster:

```bash
outpost cluster up --driver k3d
```

The driver flag updates the project configuration. Changing drivers never deletes an existing cluster automatically:

```bash
outpost cluster down
outpost cluster up --driver k3d
```

`outpost open` writes the tunneled, project-specific kubeconfig to `.outpost/kubeconfig`. It does not modify `~/.kube/config`. `outpost cluster env -- COMMAND` runs a local command with that file as `KUBECONFIG`.

Kubernetes is project-scoped. Use `outpost cluster up/down/status` and `outpost cluster env -- kubectl ...`.

## Linux machines

System containers are lightweight and work on most hosts, including standard EC2 instances. Defaults are minimal — sized for quick test environments on small VPS plans. Increase resources with `--cpu`, `--memory`, and `--disk` when you need more:


| Resource | Container default | VM default |
| -------- | ----------------- | ---------- |
| CPU      | 0.5 core          | 1 core     |
| Memory   | 128 MiB           | 256 MiB    |
| Disk     | 2 GiB             | 3 GiB      |


**Containers vs VMs:** A **system container** shares the host Linux kernel (like a very isolated chroot). It starts fast, uses little RAM, and works on almost any Linux host — this is the default. A **VM** runs a full guest kernel via KVM with stronger isolation, but needs more resources and only works when the host has KVM (bare metal, metal EC2, or nested virtualization). Use containers for everyday dev/test; use VMs when you need a real kernel or kernel modules.

Each project owns one Incus machine. Outpost checks host capacity **before** creating it. If the host is low on resources, the command fails with available amounts — run `outpost capacity` to inspect the host, or request a smaller machine.

```bash
outpost machine up --image ubuntu:24.04
outpost machine up --cpu 2 --memory 2GiB --disk 20GiB
outpost machine status
outpost machine shell
outpost machine exec -- uname -a
outpost machine copy ./app project:/tmp/app
outpost machine copy project:/tmp/output.log ./output.log
outpost machine connect --port 8080:80
outpost machine snapshot create
outpost machine down
```

**Virtual machines** need KVM. They work on bare-metal servers, metal EC2 instance types, or hosts with [nested virtualization](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/nested-virtualization.html). Standard `t3.`* instances do not support VMs — use system containers instead. VMs typically need more memory than the default; set `--memory` explicitly.

```bash
outpost host capabilities
outpost machine up --image ubuntu:24.04 --virtual-machine --cpu 2 --memory 2GiB --disk 20GiB
```



## Monitoring and cleanup

```bash
outpost status          # host health and workload summary
outpost top             # live container CPU and memory
outpost top --watch
outpost capacity        # free resources and recommendations
outpost disk            # disk usage and reclaimable space
outpost cleanup         # clean project-owned artifacts safely

outpost prune --dry-run # preview cleanup
outpost prune           # remove stopped containers, unused images, build cache
outpost prune volumes   # explicit: unused named volumes
outpost prune clusters  # owner only
outpost prune machines  # owner only
```



## Configuration

Outpost stores configuration in two places:


| Location | Scope | Purpose |
| -------- | ----- | ------- |
| `~/.outpost/` | Your machine (global) | Registered hosts, SSH keys, active host, port-forward sessions, volume archives |
| `.outpost/` | Each repository (local) | Project name, host override, compose files, environment, cleanup, sync ignore rules |

### Global config (`~/.outpost/`)

Created automatically on first use. You normally do not edit these by hand.

| File / directory | Purpose |
| ---------------- | ------- |
| `config.yaml` | Registered hosts, active host, AWS defaults |
| `identities/` | SSH keys generated for cloud hosts |
| `sessions/` | Active port-forward session metadata |
| `archives/` | Exported Docker volume backups |
| `sync-state/` | Local fingerprints used to skip redundant syncs |

### Project config (`.outpost/` in your repo)

Created by `outpost init`. Not uploaded to the remote host.

| File | Purpose |
| ---- | ------- |
| `project.yaml` | Per-repo project, remote environment, Kubernetes driver, cleanup, and compose settings |
| `.outpostignore` | Extra ignore rules for sync |


Example project config (created by `outpost init`):

```yaml
name: my-api
host: personal
remote_dir: /var/lib/outpost/projects/my-api
compose_files:
  - docker-compose.yml
environment:
  image: node:22-bookworm
  workdir: /workspace
  docker_socket: true
cleanup:
  log_retention_days: 7
  build_cache_days: 14
```

Use the same project name across your team so everyone targets the same remote stack.

## Command-line options

These flags work on every command:


| Flag          | Description                                   |
| ------------- | --------------------------------------------- |
| `--host NAME` | Use a specific host instead of the active one |
| `--json`      | JSON output                                   |
| `--debug`     | Verbose logging                               |
| `--yes`       | Skip confirmation prompts                     |




## Troubleshooting


| Problem                  | What to try                                                                                                               |
| ------------------------ | ------------------------------------------------------------------------------------------------------------------------- |
| SSH connection fails     | Test with `ssh user@host`. Check hostname, user, port, and key. Pass `--identity-file` to `host add` if needed.           |
| Bootstrap fails          | Ensure your user has `sudo` on the host. On unsupported distros, install Docker manually, then run `outpost host verify`. |
| Port forwarding conflict | Stop the local process on that port, or change port mappings in Compose or your service config. |
| Member access denied     | Owner runs `outpost invite list` and approves the device.                                                                 |
| Not enough resources     | Run `outpost capacity` before creating stacks, clusters, or machines.                                                     |
| Start over locally       | Run `outpost reset` to clear `~/.outpost` (hosts, keys, sessions). Remote servers and repo project files are kept.        |

## License

Outpost is open source software licensed under the [MIT License](LICENSE).
