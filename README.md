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
| `host verify`, `host list` | Provider and cloud lifecycle commands |

Destructive commands (`compose down`, `docker rm`, `docker system prune`, etc.) warn when other approved devices may be affected.

## Development

```bash
make build
make test
```
