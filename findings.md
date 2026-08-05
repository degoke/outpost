# Outpost AWS host findings

Date: 2026-08-05  
Binary: `bin/outpost` (executable generated binary)  
AWS profile: `skye`  
AWS account/region: `909834274473` / `us-east-2`

## Outcome

The generated binary successfully provisioned and bootstrapped an EC2 host,
ran the `node-postgres-redis` example, and exercised the applicable host,
Compose, Docker, application, cluster, machine, lifecycle, forwarding, and
cleanup commands. The temporary AWS resources were destroyed afterward.

## Provisioning

Command:

```text
./bin/outpost provider login aws --profile skye --region us-east-2
./bin/outpost host create ske-example --provider aws --profile skye --region us-east-2 --yes
```

Observed:

- Provider login succeeded for account `909834274473`.
- The first host provisioned successfully but lost SSH-key authentication after
bootstrap. It was terminated and recreated for a clean reproduction.
- The recreated host bootstrapped Ubuntu 24.04, Docker CE, Compose, and the
Outpost directories successfully.
- Host capabilities on `t3.medium`: Docker and Compose available; kind, k3d,
kubectl, Incus, KVM, and nested virtualization initially unavailable.



### Defect found: bootstrap corrupts the owner SSH key

On both fresh instances, `host create` completed, but a subsequent
`host verify` failed with `authentication failed ... provision.key`. Using EC2
Instance Connect through the standard `ubuntu` account showed:

```text
ssh-ed25519 <key># Outpost shared access keys
include /var/lib/outpost/share/authorized_keys
```

The cloud-init key was written without a trailing newline. The bootstrap script
then rewrote the file by concatenating a comment and include line directly onto
the key, making the key invalid. Replacing the temporary host’s file with the
same public key plus a newline restored access; `host verify` then passed.

## Host command observations

Successful commands:

```text
./bin/outpost host list
./bin/outpost host use ske-example
./bin/outpost host verify --yes
./bin/outpost host capabilities
./bin/outpost status
./bin/outpost capacity
./bin/outpost disk
./bin/outpost top
./bin/outpost host update-ssh-access ske-example --yes
./bin/outpost host resize ske-example --instance-type t3.small --yes
./bin/outpost host stop ske-example --yes
./bin/outpost host start ske-example --yes
./bin/outpost host restart ske-example --yes
```

Notable results:

- `status`, `capacity`, and `disk` returned useful host/Docker/resource data.
- `top` printed `unknown flag: --filter` from Docker but still exited 0 and
reported no running containers. This is a likely command-construction or
Docker compatibility bug.
- `update-ssh-access` detected the caller IP and replaced SSH ingress with a
`/32` rule.
- Resize, stop, start, and restart all completed; start/restart refreshed the
public DNS/IP and waited for SSH.
- `host remove` was intentionally not run because it only removes local state
and would orphan a live cloud host. `host add` was not applicable to the
cloud-managed host.



## Example project

The test used a temporary copy of `examples/node-postgres-redis`, initialized
as project `node-postgres-redis`.

```text
./bin/outpost init --no-shell --name node-postgres-redis
./bin/outpost compose up -d --build
```

Compose built the managed environment, built the application image, pulled
`postgres:16-alpine` and `redis:7-alpine`, waited for both health checks, and
started all three services. The health check passed:

```json
{"status":"ok","postgres":true,"redis":true}
```

Successful project commands included:

```text
./bin/outpost compose ps
./bin/outpost compose exec -T app node -e '...health check...'
./bin/outpost docker ps
./bin/outpost docker logs node-postgres-redis-app-1 --tail 20
./bin/outpost run -- echo run-ok
./bin/outpost app build
./bin/outpost app run --detach --port 3001:3000
./bin/outpost app logs
./bin/outpost app stop
./bin/outpost machine status
./bin/outpost machine up --image ubuntu:24.04
./bin/outpost machine exec -- uname -a
./bin/outpost machine snapshot create
./bin/outpost machine snapshot list
./bin/outpost machine down --yes
```

Project issues observed:

- Every remote command printed `cat: /var/lib/outpost/share/manifest.yaml: No such file or directory` even though bootstrap and the command itself
succeeded. The manifest appears to be expected but is not created during
host bootstrap.
- `outpost run -- node -e 'console.log("run-ok")'` reached the remote shell as
`node -e console.log("run-ok")` and failed with a shell syntax error. A
simple `outpost run -- echo run-ok` succeeded, indicating argument quoting
is not preserved for complex commands.
- `app status` failed when no standalone app container existed:
`template parsing error ... map has no entry for key "State"`.
- The standalone app started and then exited because the example requires
`DATABASE_URL` and `REDIS_URL`; `app logs` exposed the expected error.
- `open --port 3000:3000` started a background forwarder but timed out waiting
for readiness; `close` then reported no active forwarding session.
- `compose volumes list` reported `file does not exist` after the Compose
project had been torn down.



## Kubernetes and Incus

`cluster status` returned no active cluster. `cluster up --driver kind` failed
during Kubernetes tool preparation with:

```text
kubernetes tools install failed: no k3d checksum found
```

The failure left no cluster to remove; `cluster down --yes` completed its
cleanup path and reported zero clusters pruned.

`machine status` installed Incus when needed, and the system-container path
worked. The created machine reported:

```text
type=container ... status=running image=ubuntu:24.04
Linux ... 6.17.0-1019-aws ... x86_64
```

`machine copy` failed while trying to create an already-existing `/tmp`
directory, although `machine exec` confirmed `/tmp` was writable. Snapshot
creation succeeded. Interactive `machine shell` and `machine connect` were not
run because they require an interactive terminal or a service port.

## Cleanup and verification

Executed:

```text
./bin/outpost compose down
./bin/outpost cleanup --yes
./bin/outpost prune --dry-run --yes
./bin/outpost prune volumes --dry-run --yes
./bin/outpost prune volumes --yes --force
./bin/outpost prune --yes
./bin/outpost host destroy ske-example --delete-volumes --yes
```

Final verification:

- `./bin/outpost host list` → `No hosts registered`.
- AWS `describe-instances` reports `i-0b39a4b5211f0e7e3` as `terminated`.
- AWS queries for the test host’s tagged EBS volume and security group return
no resources.
- The first failed test instance was also destroyed before the recreated run.



## Commands not exercised

`ai` was not run because it requires an external AI-agent credential and would
not add host/runtime coverage. `invite`, `migrate`, `reset`, and `host add`
require a separate sharing, migration, local-reset, or non-cloud-host setup.
Interactive `shell` and `machine shell` were omitted to avoid hanging the
automated run. The generated command help and source were inspected for these
surfaces.