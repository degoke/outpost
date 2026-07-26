# Outpost

Outpost turns a remote Linux host into a shared Docker development environment controlled from your local terminal. No local Docker, Kubernetes, or VM software required.

## Install

```bash
go install ./cmd/outpost
```

## Quick start

```bash
# Register a remote host
outpost host add personal --hostname 203.0.113.10 --user ubuntu

# Verify connectivity and bootstrap Docker remotely
outpost host verify

# Initialize a repository with docker-compose.yml
outpost init

# Start the stack remotely
outpost compose up -d

# Run docker on the remote host
outpost docker ps

# Forward Compose ports to localhost
outpost connect
outpost connect --port 9090:80
outpost connect --status
outpost connect --down
```

Forwarded services are available at documented localhost endpoints, for example `http://127.0.0.1:8080` when port 8080 is published.

### Port forwarding troubleshooting

If `outpost connect` fails with a port conflict, either stop the local process using that port or override it:

```bash
outpost connect --local-port 18080
outpost connect --port 9090:80
```

## Resource visibility and cleanup

```bash
outpost status
outpost top
outpost top --watch
outpost capacity
outpost disk
outpost host capabilities
outpost prune --dry-run
outpost prune
outpost prune volumes --dry-run
outpost prune volumes --yes
```

## Configuration

- Global config: `~/.outpost/config.yaml`
- Project config: `.outpost/project.yaml`

## Sharing

```bash
# Owner creates an invitation
outpost invite create

# Invitee joins (requires SSH access for registration)
outpost invite join CODE --hostname HOST --user ubuntu --label my-laptop
# or using a pre-configured host entry for registration SSH
outpost invite join CODE --host personal --label my-laptop

# Owner approves
outpost invite list
outpost invite approve DEVICE_ID

# Revoke access
outpost invite revoke DEVICE_ID
```

Approved device keys are written to `/var/lib/outpost/share/authorized_keys` on the remote host.

### Member permissions

Collaborators with approved device access (role `member`) can use runtime and inspection commands but cannot manage hosts or invitations.

| Allowed | Blocked |
|---------|---------|
| `docker`, `compose`, `connect` | `host add`, `host use`, `host remove`, `host destroy` |
| `status`, `top`, `capacity`, `disk`, `prune` | `init`, `invite create/list/approve/revoke` |
| `host verify`, `host list` | `host create/start/stop/restart/resize`, `provider login` |
| `cluster list`, `cluster status`, `kubectl` | `cluster create`, `cluster delete`, `prune clusters` |
| `machine list`, `machine status`, `machine shell`, `machine exec`, `machine start/stop/restart` | `machine create`, `machine delete`, `prune machines` |
| `machine snapshot create` | `machine snapshot delete` |

Destructive commands (`compose down`, `docker rm`, `docker system prune`, etc.) warn when other approved devices may be affected.

## AWS host provisioning

```bash
outpost provider login aws --profile my-profile --region eu-west-1
outpost host create personal --provider aws --region eu-west-1
outpost host start personal
outpost host stop personal
outpost host restart personal
outpost host resize personal --instance-type t3.large
outpost host destroy personal
```

`host remove` drops local configuration only; `host destroy` terminates the EC2 instance (with optional `--delete-volumes`).

## Kubernetes clusters (kind)

```bash
outpost cluster create dev
outpost cluster create staging --workers 2
outpost cluster list
outpost cluster status dev
outpost kubectl --cluster dev get nodes
outpost kubectl --cluster dev apply -f ./manifest.yaml
outpost cluster delete dev
outpost prune clusters --dry-run
```

## Linux machines (Incus)

System containers are the default — lightweight Linux environments with a shared kernel:

```bash
outpost machine create ubuntu-dev --image ubuntu:24.04
outpost machine list
outpost machine status ubuntu-dev
outpost machine shell ubuntu-dev
outpost machine exec ubuntu-dev -- uname -a
outpost machine start ubuntu-dev
outpost machine stop ubuntu-dev
outpost machine snapshot create ubuntu-dev
outpost machine delete ubuntu-dev
outpost prune machines --dry-run
```

Hardware-virtualized VMs require KVM support on the host:

```bash
outpost host capabilities   # check vm: available/unavailable
outpost machine create vm-dev --image ubuntu:24.04 --virtual-machine --cpu 2 --memory 4GiB --disk 20GiB
```

### VM support on cloud hosts

- **Default EC2 instance types** (for example `t3.*`) do not expose KVM — use system containers instead.
- **Bare-metal instance types** (for example `*.metal`) or hosts with [nested virtualization enabled on AWS](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/nested-virtualization.html) can run `--virtual-machine`.
- **Adopted bare-metal or home servers** with `/dev/kvm` available support VMs when Incus is installed.

If VM support is unavailable, `outpost machine create --virtual-machine` fails before creating any resources and suggests creating a system container or choosing a compatible host.

## Development

```bash
make build
make test
```
