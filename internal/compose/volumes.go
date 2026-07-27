package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/goke/outpost/internal/authz"
	"github.com/goke/outpost/internal/config"
	"github.com/goke/outpost/internal/transport"
	"github.com/goke/outpost/internal/upload"
	"gopkg.in/yaml.v3"
)

const volumeStagingDir = ".volume-staging"

// VolumeInfo describes a named Docker Compose volume for a project.
type VolumeInfo struct {
	LogicalName string `json:"logical_name"`
	DockerName  string `json:"docker_name"`
}

// VolumeStatus reports local archive and remote volume state.
type VolumeStatus struct {
	VolumeInfo
	OnHost       bool   `json:"on_host"`
	EmptyOnHost  bool   `json:"empty_on_host,omitempty"`
	HasArchive   bool   `json:"has_archive"`
	ArchiveBytes int64  `json:"archive_bytes,omitempty"`
	ArchivePath  string `json:"archive_path,omitempty"`
}

type VolumeOptions struct {
	VolumeName string
	Force      bool
	ForceYes   bool
}

func composeYAMLFiles(proj *config.Project) []string {
	return upload.AllComposeFiles(proj)
}

func ParseNamedVolumes(cwd string, proj *config.Project) ([]VolumeInfo, error) {
	if proj == nil {
		return nil, fmt.Errorf("project is required")
	}
	seen := map[string]VolumeInfo{}
	for _, rel := range composeYAMLFiles(proj) {
		path := filepath.Join(cwd, rel)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", rel, err)
		}
		var doc map[string]any
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("parse %s: %w", rel, err)
		}
		vols, _ := doc["volumes"].(map[string]any)
		for logical, raw := range vols {
			if isExternalVolume(raw) {
				continue
			}
			info := VolumeInfo{
				LogicalName: logical,
				DockerName:  dockerVolumeName(proj.Name, logical, volumeExplicitName(raw)),
			}
			seen[logical] = info
		}
	}
	return sortVolumes(mapValues(seen)), nil
}

func ResolveVolumes(ctx context.Context, exec transport.Executor, cwd string, proj *config.Project) ([]VolumeInfo, error) {
	local, err := ParseNamedVolumes(cwd, proj)
	if err != nil {
		return nil, err
	}
	if exec == nil || len(local) == 0 {
		return local, nil
	}
	remote, err := resolveVolumesFromComposeConfig(ctx, exec, proj)
	if err != nil {
		return local, nil
	}
	byLogical := map[string]VolumeInfo{}
	for _, v := range local {
		byLogical[v.LogicalName] = v
	}
	for _, v := range remote {
		if _, ok := byLogical[v.LogicalName]; ok {
			byLogical[v.LogicalName] = v
		}
	}
	return sortVolumes(mapValues(byLogical)), nil
}

func resolveVolumesFromComposeConfig(ctx context.Context, exec transport.Executor, proj *config.Project) ([]VolumeInfo, error) {
	cmd := fmt.Sprintf("docker compose -p %s %s config --format json",
		shellQuote(proj.Name),
		upload.RemoteComposeArgs(proj),
	)
	var out bytes.Buffer
	code, err := exec.Run(ctx, cmd, transport.RunOpts{Stdout: &out})
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("docker compose config failed (exit %d)", code)
	}
	var doc struct {
		Volumes map[string]struct {
			Name     string `json:"name"`
			External any    `json:"external"`
		} `json:"volumes"`
	}
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		return nil, err
	}
	var vols []VolumeInfo
	for logical, spec := range doc.Volumes {
		if spec.External != nil {
			continue
		}
		dockerName := strings.TrimSpace(spec.Name)
		if dockerName == "" {
			dockerName = dockerVolumeName(proj.Name, logical, "")
		}
		vols = append(vols, VolumeInfo{LogicalName: logical, DockerName: dockerName})
	}
	if len(vols) == 0 {
		return nil, fmt.Errorf("no volumes in compose config")
	}
	return sortVolumes(vols), nil
}

func prepareVolumeOps(ctx context.Context, exec transport.Executor, cwd string, proj *config.Project) ([]VolumeInfo, error) {
	if err := upload.SyncProject(cwd, proj, exec); err != nil {
		return nil, err
	}
	return ResolveVolumes(ctx, exec, cwd, proj)
}

func isExternalVolume(raw any) bool {
	m, ok := raw.(map[string]any)
	if !ok || m == nil {
		return false
	}
	if ext, ok := m["external"].(bool); ok && ext {
		return true
	}
	if ext, ok := m["external"].(string); ok && strings.EqualFold(strings.TrimSpace(ext), "true") {
		return true
	}
	if ext, ok := m["external"].(map[string]any); ok && ext != nil {
		return true
	}
	return false
}

func volumeExplicitName(raw any) string {
	m, ok := raw.(map[string]any)
	if !ok || m == nil {
		return ""
	}
	name, _ := m["name"].(string)
	return strings.TrimSpace(name)
}

func dockerVolumeName(projectName, logicalName, explicitName string) string {
	if explicitName != "" {
		return explicitName
	}
	return projectName + "_" + logicalName
}

func remoteStagingDir(proj *config.Project) string {
	return proj.RemoteDir + "/" + volumeStagingDir
}

func localArchivePath(projectName, logicalName string) (string, error) {
	dir, err := config.VolumeArchivesDir(projectName)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, logicalName+".tar.gz"), nil
}

func volumeExists(ctx context.Context, exec transport.Executor, dockerName string) (bool, error) {
	cmd := fmt.Sprintf("docker volume inspect %s >/dev/null 2>&1", shellQuote(dockerName))
	code, err := exec.Run(ctx, cmd, transport.RunOpts{})
	if err != nil {
		return false, err
	}
	return code == 0, nil
}

func volumeIsEmpty(ctx context.Context, exec transport.Executor, dockerName string) (bool, error) {
	exists, err := volumeExists(ctx, exec, dockerName)
	if err != nil {
		return false, err
	}
	if !exists {
		return true, nil
	}
	cmd := fmt.Sprintf(
		"docker run --rm -v %s:/data:ro alpine sh -c 'if [ -z \"$(ls -A /data 2>/dev/null)\" ]; then exit 0; else exit 1; fi'",
		shellQuote(dockerName),
	)
	code, err := exec.Run(ctx, cmd, transport.RunOpts{})
	if err != nil {
		return false, err
	}
	return code == 0, nil
}

func ListVolumeStatus(ctx context.Context, exec transport.Executor, cwd string, proj *config.Project) ([]VolumeStatus, error) {
	vols, err := prepareVolumeOps(ctx, exec, cwd, proj)
	if err != nil {
		return nil, err
	}
	out := make([]VolumeStatus, 0, len(vols))
	for _, v := range vols {
		st := VolumeStatus{VolumeInfo: v}
		exists, err := volumeExists(ctx, exec, v.DockerName)
		if err != nil {
			return nil, err
		}
		st.OnHost = exists
		if exists {
			empty, err := volumeIsEmpty(ctx, exec, v.DockerName)
			if err != nil {
				return nil, err
			}
			st.EmptyOnHost = empty
		} else {
			st.EmptyOnHost = true
		}
		archive, err := localArchivePath(proj.Name, v.LogicalName)
		if err != nil {
			return nil, err
		}
		st.ArchivePath = archive
		if info, err := os.Stat(archive); err == nil {
			st.HasArchive = true
			st.ArchiveBytes = info.Size()
		}
		out = append(out, st)
	}
	return out, nil
}

func ExportVolumes(ctx context.Context, exec transport.Executor, cwd string, proj *config.Project, hostName string, opts VolumeOptions) error {
	vols, err := prepareVolumeOps(ctx, exec, cwd, proj)
	if err != nil {
		return err
	}
	if len(vols) == 0 {
		return fmt.Errorf("no named compose volumes found in project %q", proj.Name)
	}
	if opts.VolumeName != "" {
		vols = filterVolumes(vols, opts.VolumeName)
		if len(vols) == 0 {
			return fmt.Errorf("volume %q not found in compose project", opts.VolumeName)
		}
	}

	staging := remoteStagingDir(proj)
	if err := transport.EnsureRemoteDir(exec, staging); err != nil {
		return err
	}
	archiveDir, err := config.VolumeArchivesDir(proj.Name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(archiveDir, 0700); err != nil {
		return err
	}

	exported := 0
	for _, v := range vols {
		exists, err := volumeExists(ctx, exec, v.DockerName)
		if err != nil {
			return err
		}
		if !exists {
			fmt.Fprintf(os.Stderr, "skip %s: volume %s does not exist on host\n", v.LogicalName, v.DockerName)
			continue
		}
		empty, err := volumeIsEmpty(ctx, exec, v.DockerName)
		if err != nil {
			return err
		}
		if empty {
			fmt.Fprintf(os.Stderr, "skip %s: volume %s is empty\n", v.LogicalName, v.DockerName)
			continue
		}
		remoteArchive := staging + "/" + v.LogicalName + ".tar.gz"
		exportCmd := fmt.Sprintf(
			"docker run --rm -v %s:/from:ro -v %s:/to alpine tar czf /to/%s -C /from .",
			shellQuote(v.DockerName),
			shellQuote(staging),
			shellQuote(v.LogicalName+".tar.gz"),
		)
		if code, err := exec.Run(ctx, exportCmd, transport.RunOpts{}); err != nil {
			return err
		} else if code != 0 {
			return fmt.Errorf("export volume %s failed (exit %d)", v.LogicalName, code)
		}
		data, err := exec.Download(remoteArchive)
		if err != nil {
			return fmt.Errorf("download archive for %s: %w", v.LogicalName, err)
		}
		localPath, err := localArchivePath(proj.Name, v.LogicalName)
		if err != nil {
			return err
		}
		tmpPath := localPath + ".tmp"
		if err := os.WriteFile(tmpPath, data, 0600); err != nil {
			return err
		}
		if err := os.Rename(tmpPath, localPath); err != nil {
			return err
		}
		exported++
		fmt.Fprintf(os.Stderr, "exported %s -> %s\n", v.DockerName, localPath)
	}
	if exported == 0 {
		return fmt.Errorf("no volumes exported")
	}
	return updateVolumeState(cwd, proj, hostName)
}

func ImportVolumes(ctx context.Context, exec transport.Executor, cwd string, proj *config.Project, hostName string, opts VolumeOptions) error {
	vols, err := prepareVolumeOps(ctx, exec, cwd, proj)
	if err != nil {
		return err
	}
	if len(vols) == 0 {
		return fmt.Errorf("no named compose volumes found in project %q", proj.Name)
	}
	if opts.VolumeName != "" {
		vols = filterVolumes(vols, opts.VolumeName)
		if len(vols) == 0 {
			return fmt.Errorf("volume %q not found in compose project", opts.VolumeName)
		}
	}

	staging := remoteStagingDir(proj)
	if err := transport.EnsureRemoteDir(exec, staging); err != nil {
		return err
	}

	imported := 0
	for _, v := range vols {
		ok, err := importOneVolume(ctx, exec, staging, proj.Name, v, opts.Force)
		if err != nil {
			return err
		}
		if ok {
			imported++
		}
	}
	if imported == 0 {
		return fmt.Errorf("no volumes imported")
	}
	return updateVolumeState(cwd, proj, hostName)
}

func importOneVolume(ctx context.Context, exec transport.Executor, staging string, projectName string, v VolumeInfo, force bool) (bool, error) {
	localPath, err := localArchivePath(projectName, v.LogicalName)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(localPath); err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "skip %s: no local archive at %s\n", v.LogicalName, localPath)
			return false, nil
		}
		return false, err
	}
	exists, err := volumeExists(ctx, exec, v.DockerName)
	if err != nil {
		return false, err
	}
	if exists && !force {
		empty, err := volumeIsEmpty(ctx, exec, v.DockerName)
		if err != nil {
			return false, err
		}
		if !empty {
			fmt.Fprintf(os.Stderr, "skip %s: volume %s already exists on host (use --force to overwrite)\n", v.LogicalName, v.DockerName)
			return false, nil
		}
	}
	if exists {
		rmCmd := fmt.Sprintf("docker volume rm %s", shellQuote(v.DockerName))
		if code, err := exec.Run(ctx, rmCmd, transport.RunOpts{}); err != nil {
			return false, err
		} else if code != 0 {
			return false, fmt.Errorf("remove existing volume %s failed (exit %d) — stop containers using it first", v.DockerName, code)
		}
	}
	createCmd := fmt.Sprintf("docker volume create %s", shellQuote(v.DockerName))
	if code, err := exec.Run(ctx, createCmd, transport.RunOpts{}); err != nil {
		return false, err
	} else if code != 0 {
		return false, fmt.Errorf("create volume %s failed (exit %d)", v.DockerName, code)
	}
	remoteArchive := staging + "/" + v.LogicalName + ".tar.gz"
	if err := exec.Upload(localPath, remoteArchive); err != nil {
		return false, fmt.Errorf("upload archive for %s: %w", v.LogicalName, err)
	}
	importCmd := fmt.Sprintf(
		"docker run --rm -v %s:/to -v %s:/from:ro alpine tar xzf /from/%s -C /to",
		shellQuote(v.DockerName),
		shellQuote(staging),
		shellQuote(v.LogicalName+".tar.gz"),
	)
	if code, err := exec.Run(ctx, importCmd, transport.RunOpts{}); err != nil {
		return false, err
	} else if code != 0 {
		return false, fmt.Errorf("import volume %s failed (exit %d)", v.LogicalName, code)
	}
	fmt.Fprintf(os.Stderr, "imported %s -> %s\n", localPath, v.DockerName)
	return true, nil
}

// EnsureImported restores missing project volumes from local archives before compose up.
func EnsureImported(ctx context.Context, exec transport.Executor, cwd string, proj *config.Project, hostName string, forceYes bool) error {
	vols, err := ResolveVolumes(ctx, exec, cwd, proj)
	if err != nil {
		return err
	}
	if len(vols) == 0 {
		return nil
	}

	var pending []VolumeInfo
	for _, v := range vols {
		localPath, err := localArchivePath(proj.Name, v.LogicalName)
		if err != nil {
			return err
		}
		if _, err := os.Stat(localPath); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		needsImport, err := volumeNeedsImport(ctx, exec, v.DockerName)
		if err != nil {
			return err
		}
		if needsImport {
			pending = append(pending, v)
		}
	}
	if len(pending) == 0 {
		return nil
	}

	if proj.Volumes != nil && proj.Volumes.LastHost != "" && proj.Volumes.LastHost != hostName {
		fmt.Fprintf(os.Stderr, "note: importing volumes previously synced from host %q onto %q\n", proj.Volumes.LastHost, hostName)
	}

	names := make([]string, len(pending))
	for i, v := range pending {
		names[i] = v.LogicalName
	}
	msg := fmt.Sprintf("import %d project volume(s) from local archives: %s", len(pending), strings.Join(names, ", "))
	if err := authz.ConfirmWithYes(msg, forceYes); err != nil {
		return err
	}

	staging := remoteStagingDir(proj)
	if err := transport.EnsureRemoteDir(exec, staging); err != nil {
		return err
	}
	imported := 0
	for _, v := range pending {
		ok, err := importOneVolume(ctx, exec, staging, proj.Name, v, false)
		if err != nil {
			return err
		}
		if ok {
			imported++
		}
	}
	if imported == 0 {
		return fmt.Errorf("no volumes imported")
	}
	return updateVolumeState(cwd, proj, hostName)
}

func volumeNeedsImport(ctx context.Context, exec transport.Executor, dockerName string) (bool, error) {
	exists, err := volumeExists(ctx, exec, dockerName)
	if err != nil {
		return false, err
	}
	if !exists {
		return true, nil
	}
	return volumeIsEmpty(ctx, exec, dockerName)
}

func filterVolumes(vols []VolumeInfo, name string) []VolumeInfo {
	name = strings.TrimSpace(name)
	var out []VolumeInfo
	for _, v := range vols {
		if v.LogicalName == name || v.DockerName == name {
			out = append(out, v)
		}
	}
	return out
}

func updateVolumeState(cwd string, proj *config.Project, hostName string) error {
	if proj.Volumes == nil {
		proj.Volumes = &config.ProjectVolumeState{}
	}
	proj.Volumes.LastHost = hostName
	proj.Volumes.LastSynced = time.Now().UTC()
	return config.SaveProject(cwd, proj)
}

func sortVolumes(vols []VolumeInfo) []VolumeInfo {
	sort.Slice(vols, func(i, j int) bool {
		return vols[i].LogicalName < vols[j].LogicalName
	})
	return vols
}

func mapValues(m map[string]VolumeInfo) []VolumeInfo {
	out := make([]VolumeInfo, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}
