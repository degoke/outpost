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

## Development

```bash
make build
make test
```
