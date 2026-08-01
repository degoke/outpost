package migrate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/degoke/outpost/internal/cluster"
	"github.com/degoke/outpost/internal/compose"
	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/inspect"
	"github.com/degoke/outpost/internal/output"
	"github.com/degoke/outpost/internal/transport"
)

const (
	dockerBundleArchive = "docker-bundle.tar"
	bundleDirName       = "docker-bundle"
)

// DockerBundleManifest describes a exported project Docker environment.
type DockerBundleManifest struct {
	Project         string         `json:"project"`
	ImagePrefix     string         `json:"image_prefix"`
	Volumes         []DockerVolume `json:"volumes"`
	KubernetesState string         `json:"kubernetes_state,omitempty"`
	ContainerCount  int            `json:"container_count"`
}

type dockerBundleOptions struct {
	SkipCluster bool
}

func exportDockerBundle(ctx context.Context, exec transport.Executor, cwd string, proj *config.Project, out *output.Printer, opts dockerBundleOptions) (bool, error) {
	removeLocalBundle(proj.Name)

	vols := projectDockerVolumes(cwd, proj)
	runtimeName := cluster.KindName(proj.Name)
	containerIDs, err := discoverProjectContainerIDs(ctx, exec, proj, runtimeName, opts.SkipCluster)
	if err != nil {
		return false, err
	}
	if len(containerIDs) == 0 && len(vols) == 0 {
		return false, nil
	}

	staging := remoteMigrateStaging(proj)
	bundleRoot := staging + "/" + bundleDirName
	volumesDir := bundleRoot + "/volumes"
	if err := transport.EnsureRemoteDir(exec, volumesDir); err != nil {
		return false, err
	}
	if out != nil {
		out.Step("Exporting project Docker environment...")
	}

	if len(containerIDs) > 0 {
		if err := stopContainerIDs(ctx, exec, containerIDs); err != nil {
			return false, err
		}
	}

	if _, err := exportDockerVolumesTo(ctx, exec, vols, volumesDir); err != nil {
		return false, err
	}

	prefix := bundleImagePrefix(proj.Name)
	remoteImages := bundleRoot + "/images.tar"
	remoteInspect := bundleRoot + "/inspect.json"
	kubernetesState := ""
	if !opts.SkipCluster && proj.Kubernetes != nil {
		stateDir := strings.TrimRight(proj.RemoteDir, "/") + "/.outpost/kubernetes"
		remoteK8s := bundleRoot + "/kubernetes-state.tar.gz"
		stateCmd := fmt.Sprintf(
			"if [ -d %s ]; then tar czf %s -C %s kubernetes; fi",
			shellQuote(stateDir),
			shellQuote(remoteK8s),
			shellQuote(strings.TrimRight(proj.RemoteDir, "/")+"/.outpost"),
		)
		if _, err := exec.Run(ctx, stateCmd, transport.RunOpts{}); err != nil {
			return false, err
		}
		if remoteFileExists(ctx, exec, remoteK8s) {
			kubernetesState = "kubernetes-state.tar.gz"
		}
	}

	if len(containerIDs) > 0 {
		ids := strings.Join(containerIDs, " ")
		saveCmd := fmt.Sprintf(`
set -e
ids="%s"
tags=""
i=0
for cid in $ids; do
  tag="%s:$i"
  docker commit "$cid" "$tag" >/dev/null
  tags="$tags $tag"
  i=$((i+1))
done
docker inspect $ids > %s
docker save -o %s $tags
`,
			ids,
			prefix,
			shellQuote(remoteInspect),
			shellQuote(remoteImages),
		)
		code, err := exec.Run(ctx, strings.TrimSpace(saveCmd), transport.RunOpts{})
		if err != nil {
			return false, err
		}
		if code != 0 {
			return false, fmt.Errorf("export docker containers failed (exit %d)", code)
		}
	} else {
		if err := exec.UploadBytes([]byte("[]"), remoteInspect); err != nil {
			return false, err
		}
	}

	manifest := DockerBundleManifest{
		Project:         proj.Name,
		ImagePrefix:     prefix,
		Volumes:         vols,
		KubernetesState: kubernetesState,
		ContainerCount:  len(containerIDs),
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return false, err
	}
	if err := exec.UploadBytes(manifestBytes, bundleRoot+"/manifest.json"); err != nil {
		return false, err
	}

	remoteBundle := staging + "/" + dockerBundleArchive
	packCmd := fmt.Sprintf("tar czf %s -C %s %s", shellQuote(remoteBundle), shellQuote(staging), bundleDirName)
	code, err := exec.Run(ctx, packCmd, transport.RunOpts{})
	if err != nil {
		return false, err
	}
	if code != 0 {
		return false, fmt.Errorf("pack docker bundle failed (exit %d)", code)
	}
	if err := downloadArchive(exec, proj.Name, dockerBundleArchive, remoteBundle); err != nil {
		return false, err
	}
	return true, nil
}

func importDockerBundle(ctx context.Context, exec transport.Executor, proj *config.Project, out *output.Printer) (*DockerBundleManifest, error) {
	localPath, err := localArchiveFile(proj.Name, dockerBundleArchive)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(localPath); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	staging := remoteMigrateStaging(proj)
	bundleRoot := staging + "/" + bundleDirName
	if err := transport.EnsureRemoteDir(exec, staging); err != nil {
		return nil, err
	}
	if out != nil {
		out.Step("Importing project Docker environment...")
	}

	data, err := os.ReadFile(localPath)
	if err != nil {
		return nil, err
	}
	remoteBundle := staging + "/" + dockerBundleArchive
	if err := exec.UploadBytes(data, remoteBundle); err != nil {
		return nil, err
	}
	extractCmd := fmt.Sprintf("rm -rf %s && tar xzf %s -C %s", shellQuote(bundleRoot), shellQuote(remoteBundle), shellQuote(staging))
	code, err := exec.Run(ctx, extractCmd, transport.RunOpts{})
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("extract docker bundle failed (exit %d)", code)
	}

	manifestData, err := exec.Download(bundleRoot + "/manifest.json")
	if err != nil {
		return nil, fmt.Errorf("read bundle manifest: %w", err)
	}
	var manifest DockerBundleManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, err
	}

	if remoteFileExists(ctx, exec, bundleRoot+"/images.tar") {
		loadCmd := fmt.Sprintf("docker load -i %s", shellQuote(bundleRoot+"/images.tar"))
		code, err = exec.Run(ctx, loadCmd, transport.RunOpts{})
		if err != nil {
			return nil, err
		}
		if code != 0 {
			return nil, fmt.Errorf("docker load bundle images failed (exit %d)", code)
		}
	}

	if remoteFileExists(ctx, exec, bundleRoot+"/inspect.json") {
		if err := recreateContainersFromInspect(ctx, exec, bundleRoot+"/inspect.json", manifest.ImagePrefix); err != nil {
			return nil, err
		}
	}

	if _, err := importDockerVolumesFrom(ctx, exec, bundleRoot+"/volumes", manifest.Volumes, true); err != nil {
		return nil, err
	}

	if manifest.KubernetesState != "" && remoteFileExists(ctx, exec, bundleRoot+"/"+manifest.KubernetesState) {
		outpostDir := strings.TrimRight(proj.RemoteDir, "/") + "/.outpost"
		k8sCmd := fmt.Sprintf(
			"mkdir -p %s && tar xzf %s -C %s",
			shellQuote(outpostDir),
			shellQuote(bundleRoot+"/"+manifest.KubernetesState),
			shellQuote(outpostDir),
		)
		code, err = exec.Run(ctx, k8sCmd, transport.RunOpts{})
		if err != nil {
			return nil, err
		}
		if code != 0 {
			return nil, fmt.Errorf("extract kubernetes state from bundle failed (exit %d)", code)
		}
	}

	return &manifest, nil
}

func removeLocalBundle(projectName string) {
	path, err := localArchiveFile(projectName, dockerBundleArchive)
	if err != nil {
		return
	}
	_ = os.Remove(path)
}

func remoteFileExists(ctx context.Context, exec transport.Executor, path string) bool {
	cmd := fmt.Sprintf("test -s %s", shellQuote(path))
	code, err := exec.Run(ctx, cmd, transport.RunOpts{})
	return err == nil && code == 0
}

func bundleExists(projectName string) bool {
	path, err := localArchiveFile(projectName, dockerBundleArchive)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

func bundleImagePrefix(projectName string) string {
	return "outpost-migrate-" + config.SanitizeProjectName(projectName)
}

func projectDockerVolumes(cwd string, proj *config.Project) []DockerVolume {
	seen := map[string]DockerVolume{}
	add := func(v DockerVolume) {
		if v.DockerName == "" {
			return
		}
		if _, ok := seen[v.DockerName]; !ok {
			seen[v.DockerName] = v
		}
	}
	for _, v := range dependencyVolumes(cwd, proj) {
		add(v)
	}
	if proj.RequireCompose() == nil {
		if vols, err := compose.ParseNamedVolumes(cwd, proj); err == nil {
			for _, v := range vols {
				add(DockerVolume{
					ArchiveName: sanitizeVolumeArchiveName(v.DockerName) + ".tar.gz",
					DockerName:  v.DockerName,
				})
			}
		}
	}
	out := make([]DockerVolume, 0, len(seen))
	for _, v := range seen {
		out = append(out, v)
	}
	return out
}

func sanitizeVolumeArchiveName(name string) string {
	name = strings.NewReplacer("/", "_", ":", "_", " ", "_").Replace(name)
	if name == "" {
		return "volume"
	}
	return "vol-" + name
}

func discoverProjectContainerIDs(ctx context.Context, exec transport.Executor, proj *config.Project, runtimeName string, skipCluster bool) ([]string, error) {
	safe := config.SanitizeProjectName(proj.Name)
	devName := "outpost-dev-" + safe
	appName := "outpost-app-" + safe
	remoteDir := shellQuote(proj.RemoteDir)
	clusterFilter := ""
	if !skipCluster {
		clusterFilter = fmt.Sprintf(
			"docker ps -aq --filter label=io.x-k8s.kind.cluster=%s 2>/dev/null; docker ps -aq --filter label=k3d.cluster=%s 2>/dev/null;",
			shellQuote(runtimeName), shellQuote(runtimeName),
		)
	}
	cmd := fmt.Sprintf(
		`(%s`+
			`docker ps -aq --filter label=com.outpost.project=%s 2>/dev/null; `+
			`docker ps -aq --filter name=^/%s$ 2>/dev/null; `+
			`docker ps -aq --filter name=^/%s$ 2>/dev/null; `+
			`(cd %s && docker compose -p %s ps -aq 2>/dev/null || true)`+
			`) | sort -u | sed '/^$/d'`,
		clusterFilter,
		shellQuote(proj.Name),
		devName, appName,
		remoteDir, shellQuote(safe),
	)
	out, err := inspect.RunOutput(ctx, exec, cmd)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			ids = append(ids, line)
		}
	}
	return ids, nil
}

func stopContainerIDs(ctx context.Context, exec transport.Executor, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	cmd := fmt.Sprintf("docker stop %s 2>/dev/null || true", strings.Join(ids, " "))
	_, err := exec.Run(ctx, cmd, transport.RunOpts{})
	return err
}

func recreateContainersFromInspect(ctx context.Context, exec transport.Executor, inspectPath, imagePrefix string) error {
	recreateCmd := fmt.Sprintf(`
set -e
python3 - <<'PY'
import json, subprocess

prefix = %q
with open(%q) as f:
    items = json.load(f)
if not isinstance(items, list):
    items = [items]

for i, item in enumerate(items):
    name = item["Name"].lstrip("/")
    image = f"{prefix}:{i}"
    config = item.get("Config", {}) or {}
    host_config = item.get("HostConfig", {}) or {}
    labels = config.get("Labels") or {}
    hostname = config.get("Hostname") or name
    cmd = config.get("Cmd")
    entrypoint = config.get("Entrypoint")
    network = host_config.get("NetworkMode") or "bridge"
    args = ["docker", "create", "--name", name, "--hostname", hostname, "--network", network]
    for k, v in labels.items():
        args.extend(["--label", f"{k}={v}"])
    for env in config.get("Env") or []:
        args.extend(["-e", env])
    if config.get("User"):
        args.extend(["--user", config["User"]])
    if config.get("WorkingDir"):
        args.extend(["-w", config["WorkingDir"]])
    restart = host_config.get("RestartPolicy") or {}
    restart_name = restart.get("Name") or ""
    if restart_name and restart_name != "no":
        max_retries = restart.get("MaximumRetryCount")
        if restart_name == "on-failure" and max_retries is not None:
            args.extend(["--restart", f"on-failure:{max_retries}"])
        else:
            args.extend(["--restart", restart_name])
    for container_port, bindings in (host_config.get("PortBindings") or {}).items():
        port = container_port.split("/")[0]
        for binding in bindings or []:
            host_port = binding.get("HostPort")
            if not host_port:
                continue
            host_ip = binding.get("HostIp") or ""
            if host_ip:
                args.extend(["-p", f"{host_ip}:{host_port}:{port}"])
            else:
                args.extend(["-p", f"{host_port}:{port}"])
    for m in item.get("Mounts") or []:
        if m.get("Type") == "bind" and m.get("Source") and m.get("Destination"):
            mode = ":ro" if not m.get("RW", True) else ""
            args.extend(["-v", f"{m['Source']}:{m['Destination']}{mode}"])
        elif m.get("Type") == "volume" and m.get("Name") and m.get("Destination"):
            args.extend(["-v", f"{m['Name']}:{m['Destination']}"])
    args.append(image)
    if entrypoint:
        args.extend(["--entrypoint", entrypoint[0] if isinstance(entrypoint, list) else str(entrypoint)])
    if cmd:
        args.extend(cmd if isinstance(cmd, list) else [cmd])
    subprocess.run(["docker", "rm", "-f", name], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    subprocess.check_call(args)
    subprocess.check_call(["docker", "start", name])
PY
`,
		imagePrefix,
		shellQuote(inspectPath),
	)
	code, err := exec.Run(ctx, strings.TrimSpace(recreateCmd), transport.RunOpts{})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("recreate docker containers failed (exit %d)", code)
	}
	return nil
}

// ProjectDockerVolumesForTest exposes bundle volume discovery for tests.
func ProjectDockerVolumesForTest(cwd string, proj *config.Project) []DockerVolume {
	return projectDockerVolumes(cwd, proj)
}

// DockerBundleExistsForTest reports whether a docker bundle archive exists.
func DockerBundleExistsForTest(projectName string) bool {
	return bundleExists(projectName)
}

// RemoveLocalBundleForTest removes any local docker bundle archive.
func RemoveLocalBundleForTest(projectName string) {
	removeLocalBundle(projectName)
}
