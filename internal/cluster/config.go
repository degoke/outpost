package cluster

import (
	"fmt"
	"strings"
)

const (
	mib = 1024 * 1024
	gib = 1024 * 1024 * 1024
)

// Dev cluster capacity reservations for the pre-create check.
//
// Values track upstream minimums for a small single-node dev cluster, plus ~25%
// headroom so checks are realistic without blocking typical dev hosts (e.g.
// 2 vCPU / 4 GiB). The host capacity layer also keeps a 10% safety margin.
//
// References:
//   - k3s server: 512 MiB RAM minimum (docs.k3s.io)
//   - kind: 2 GiB RAM minimum for a single-node cluster (kind.sigs.k8s.io)
const (
	k3dServerCPU = 0.75 // ~0.5 core k3s server + k3d loadbalancer
	k3dServerMem = 768 * mib
	k3dAgentCPU  = 0.25
	k3dAgentMem  = 384 * mib
	k3dBaseDisk  = uint64(768 * mib) // k3s image + etcd; tight dev allowance
	k3dAgentDisk = 256 * mib

	kindControlCPU = 1.25 // full node container; busier than k3s but fine for dev
	kindControlMem = 2 * gib
	kindWorkerCPU  = 0.75
	kindWorkerMem  = 1 * gib
	kindBaseDisk   = uint64(1536 * mib) // kindest/node image + small layer buffer
	kindWorkerDisk = 512 * mib
)

type KindConfig struct {
	Name          string
	ControlPlanes int
	Workers       int
}

func RenderKindConfig(cfg KindConfig) string {
	if cfg.ControlPlanes == 0 {
		cfg.ControlPlanes = 1
	}
	var b strings.Builder
	b.WriteString("kind: Cluster\n")
	b.WriteString("apiVersion: kind.x-k8s.io/v1alpha4\n")
	b.WriteString(fmt.Sprintf("name: %s\n", cfg.Name))
	// The cluster is created through the remote host Docker socket from inside
	// the project container. Binding the API port beyond the host loopback lets
	// the container reach it through host.docker.internal; outpost open still
	// tunnels the host-local endpoint to the developer's machine.
	b.WriteString("networking:\n")
	b.WriteString("  apiServerAddress: 0.0.0.0\n")
	b.WriteString("kubeadmConfigPatches:\n")
	b.WriteString("- |\n")
	b.WriteString("  kind: ClusterConfiguration\n")
	b.WriteString("  apiServer:\n")
	b.WriteString("    certSANs:\n")
	b.WriteString("    - host.docker.internal\n")
	b.WriteString("    - 127.0.0.1\n")
	b.WriteString("    - localhost\n")
	b.WriteString("    - 0.0.0.0\n")
	b.WriteString("nodes:\n")
	for i := 0; i < cfg.ControlPlanes; i++ {
		b.WriteString("- role: control-plane\n")
	}
	for i := 0; i < cfg.Workers; i++ {
		b.WriteString("- role: worker\n")
	}
	return b.String()
}

func EstimateResources(driver Driver, controlPlanes, workers int) (cpu float64, memBytes, diskBytes uint64) {
	if controlPlanes == 0 {
		controlPlanes = 1
	}
	switch driver {
	case DriverK3d:
		cpu = float64(controlPlanes)*k3dServerCPU + float64(workers)*k3dAgentCPU
		memBytes = uint64(controlPlanes)*k3dServerMem + uint64(workers)*k3dAgentMem
		diskBytes = k3dBaseDisk + uint64(workers)*k3dAgentDisk
	default:
		cpu = float64(controlPlanes)*kindControlCPU + float64(workers)*kindWorkerCPU
		memBytes = uint64(controlPlanes)*kindControlMem + uint64(workers)*kindWorkerMem
		diskBytes = kindBaseDisk + uint64(workers)*kindWorkerDisk
	}
	return cpu, memBytes, diskBytes
}
