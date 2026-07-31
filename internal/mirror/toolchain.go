package mirror

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/environment"
	"github.com/degoke/outpost/internal/output"
	"github.com/degoke/outpost/internal/transport"
)

const toolchainsBase = config.DefaultRemoteBase + "/toolchains"

// ToolchainPlan describes packages and runtimes required for a mirrored project.
type ToolchainPlan struct {
	Packages  []string `json:"packages,omitempty"`
	GoVersion string   `json:"go_version,omitempty"`
}

var allowedPackages = map[string]bool{
	"make":            true,
	"git":             true,
	"curl":            true,
	"ca-certificates": true,
	"python3":         true,
	"python3-venv":    true,
	"build-essential": true,
}

var (
	goToolchainRE  = regexp.MustCompile(`(?m)^toolchain\s+go(\d+\.\d+(?:\.\d+)?)`)
	goVersionRE    = regexp.MustCompile(`(?m)^go\s+(\d+\.\d+(?:\.\d+)?)`)
	goInMakefileRE = regexp.MustCompile(`(^|\s)go\s+(build|run|test|install|generate|mod|get)\b`)
)

// DetectPlan merges explicit project toolchain config with repo markers and command hints.
func DetectPlan(cwd string, proj *config.Project, command string) (ToolchainPlan, error) {
	var plan ToolchainPlan
	if proj != nil && proj.Toolchain != nil {
		plan.Packages = append(plan.Packages, proj.Toolchain.Packages...)
		plan.GoVersion = strings.TrimSpace(proj.Toolchain.Go)
	}
	if err := mergeDetected(&plan, cwd); err != nil {
		return ToolchainPlan{}, err
	}
	mergeCommandHints(&plan, command)
	if err := normalizePlan(&plan); err != nil {
		return ToolchainPlan{}, err
	}
	return plan, nil
}

func mergeDetected(plan *ToolchainPlan, cwd string) error {
	for _, name := range []string{"Makefile", "makefile", "GNUmakefile"} {
		path := filepath.Join(cwd, name)
		if !fileExists(path) {
			continue
		}
		plan.Packages = append(plan.Packages, "make")
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if goInMakefileRE.Match(data) && plan.GoVersion == "" {
			if fileExists(filepath.Join(cwd, "go.mod")) {
				v, err := parseGoModVersion(filepath.Join(cwd, "go.mod"))
				if err != nil {
					return err
				}
				plan.GoVersion = v
			} else {
				plan.GoVersion = "1.22.5"
			}
		}
	}
	if fileExists(filepath.Join(cwd, "go.mod")) {
		v, err := parseGoModVersion(filepath.Join(cwd, "go.mod"))
		if err != nil {
			return err
		}
		if plan.GoVersion == "" {
			plan.GoVersion = v
		}
	}
	return nil
}

func mergeCommandHints(plan *ToolchainPlan, command string) {
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return
	}
	switch fields[0] {
	case "make":
		plan.Packages = append(plan.Packages, "make")
	case "go":
		if plan.GoVersion == "" {
			plan.GoVersion = "1.22.5"
		}
	}
}

func normalizePlan(plan *ToolchainPlan) error {
	expandPlanDependencies(plan)
	plan.Packages = uniqueSorted(plan.Packages)
	for _, pkg := range plan.Packages {
		if !allowedPackages[pkg] {
			return fmt.Errorf("unsupported toolchain package %q (allowed: make, git, curl, ca-certificates, python3, python3-venv, build-essential)", pkg)
		}
	}
	plan.GoVersion = normalizeGoVersion(plan.GoVersion)
	return nil
}

func expandPlanDependencies(plan *ToolchainPlan) {
	if plan.GoVersion == "" {
		return
	}
	plan.Packages = append(plan.Packages, "curl", "ca-certificates")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func parseGoModVersion(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	content := string(data)
	if m := goToolchainRE.FindStringSubmatch(content); len(m) == 2 {
		return normalizeGoVersion(m[1]), nil
	}
	if m := goVersionRE.FindStringSubmatch(content); len(m) == 2 {
		return normalizeGoVersion(m[1]), nil
	}
	return "1.22.5", nil
}

func normalizeGoVersion(v string) string {
	v = strings.TrimSpace(strings.TrimPrefix(v, "go"))
	if v == "" {
		return ""
	}
	parts := strings.Split(v, ".")
	if len(parts) == 2 {
		return v + ".0"
	}
	return v
}

func uniqueSorted(items []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func (p ToolchainPlan) Empty() bool {
	return len(p.Packages) == 0 && p.GoVersion == ""
}

// EnsureToolchain installs missing packages and runtimes. Returns PATH prefixes to prepend.
func EnsureToolchain(ctx context.Context, exec transport.Executor, plan ToolchainPlan, out *output.Printer) ([]string, error) {
	if plan.Empty() {
		return nil, nil
	}
	var pathPrefixes []string
	if err := ensurePackages(ctx, exec, plan.Packages, out); err != nil {
		return nil, err
	}
	if plan.GoVersion != "" {
		goBin, err := ensureGo(ctx, exec, plan.GoVersion, out)
		if err != nil {
			return nil, err
		}
		pathPrefixes = append(pathPrefixes, filepath.Dir(goBin))
	}
	return pathPrefixes, nil
}

func ensurePackages(ctx context.Context, exec transport.Executor, packages []string, out *output.Printer) error {
	var missing []string
	for _, pkg := range packages {
		check := packagePresenceCheck(pkg)
		code, err := exec.Run(ctx, check, transport.RunOpts{})
		if err != nil {
			return err
		}
		if code != 0 {
			missing = append(missing, pkg)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	if out != nil {
		out.Step("Installing toolchain packages: %s", strings.Join(missing, ", "))
	}
	script := packageInstallScript(missing)
	var stderr strings.Builder
	code, err := exec.Run(ctx, script, transport.RunOpts{Stderr: &stderr})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("package install failed: %s", strings.TrimSpace(stderr.String()))
	}
	return nil
}

func packageInstallScript(packages []string) string {
	aptList := strings.Join(quoteList(aptPackages(packages)), " ")
	yumList := strings.Join(quoteList(yumPackages(packages)), " ")
	return fmt.Sprintf(`
set -e
need_sudo=""
if [ "$(id -u)" -ne 0 ]; then need_sudo="sudo"; fi
if command -v apt-get >/dev/null 2>&1; then
  $need_sudo apt-get update -qq && $need_sudo apt-get install -y -qq %s
elif command -v yum >/dev/null 2>&1; then
  $need_sudo yum install -y -q %s
else
  echo "OUTPOST_ERROR: no supported package manager found"
  exit 1
fi
`, aptList, yumList)
}

func aptPackages(packages []string) []string {
	return append([]string(nil), packages...)
}

func yumPackages(packages []string) []string {
	var out []string
	for _, pkg := range packages {
		switch pkg {
		case "build-essential":
			out = append(out, "gcc", "gcc-c++", "make")
		case "python3-venv":
			out = append(out, "python3")
		default:
			out = append(out, pkg)
		}
	}
	return uniqueSorted(out)
}

func quoteList(items []string) []string {
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = shellQuote(item)
	}
	return quoted
}

func packagePresenceCheck(pkg string) string {
	switch pkg {
	case "build-essential":
		return `command -v gcc >/dev/null 2>&1 && command -v make >/dev/null 2>&1`
	case "python3-venv":
		return `python3 -m venv -h >/dev/null 2>&1`
	case "ca-certificates":
		return `dpkg -s ca-certificates >/dev/null 2>&1 || rpm -q ca-certificates >/dev/null 2>&1`
	}
	if bin, ok := packageBinaries[pkg]; ok {
		return fmt.Sprintf("command -v %s >/dev/null 2>&1", bin)
	}
	return fmt.Sprintf("dpkg -s %s >/dev/null 2>&1 || rpm -q %s >/dev/null 2>&1", pkg, pkg)
}

var packageBinaries = map[string]string{
	"make":    "make",
	"git":     "git",
	"curl":    "curl",
	"python3": "python3",
}

func ensureGo(ctx context.Context, exec transport.Executor, version string, out *output.Printer) (string, error) {
	installDir := filepath.Join(toolchainsBase, "go", version)
	goBin := filepath.Join(installDir, "bin", "go")
	check := fmt.Sprintf("test -x %s", shellQuote(goBin))
	code, err := exec.Run(ctx, check, transport.RunOpts{})
	if err != nil {
		return "", err
	}
	if code == 0 {
		return goBin, nil
	}
	if out != nil {
		out.Step("Installing Go %s...", version)
	}
	arch, err := remoteGoArch(ctx, exec)
	if err != nil {
		return "", err
	}
	script := goInstallScript(version, arch, installDir)
	var stderr strings.Builder
	code, err = exec.Run(ctx, script, transport.RunOpts{Stderr: &stderr})
	if err != nil {
		return "", err
	}
	if code != 0 {
		msg := strings.TrimSpace(stderr.String())
		if strings.Contains(msg, "OUTPOST_ERROR:") {
			return "", fmt.Errorf("%s", strings.TrimSpace(strings.Split(msg, "OUTPOST_ERROR:")[1]))
		}
		return "", fmt.Errorf("go install failed: %s", msg)
	}
	return goBin, nil
}

func remoteGoArch(ctx context.Context, exec transport.Executor) (string, error) {
	var out strings.Builder
	code, err := exec.Run(ctx, "uname -m", transport.RunOpts{Stdout: &out})
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "amd64", nil
	}
	switch strings.TrimSpace(out.String()) {
	case "aarch64", "arm64":
		return "arm64", nil
	default:
		return "amd64", nil
	}
}

func goInstallScript(version, arch, installDir string) string {
	tarball := fmt.Sprintf("go%s.linux-%s.tar.gz", version, arch)
	url := fmt.Sprintf("https://go.dev/dl/%s", tarball)
	return fmt.Sprintf(`
set -e
need_sudo=""
if [ "$(id -u)" -ne 0 ]; then need_sudo="sudo"; fi
if [ -x %s ]; then
  exit 0
fi
$need_sudo mkdir -p %s
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
curl -fsSL %s -o "$tmp/%s"
tar -C "$tmp" -xzf "$tmp/%s"
$need_sudo rm -rf %s
$need_sudo mv "$tmp/go" %s
`, shellQuote(installDir+"/bin/go"), shellQuote(filepath.Dir(installDir)), shellQuote(url), tarball, tarball, shellQuote(installDir), shellQuote(installDir))
}

func wrapCommandPath(cmd string, pathPrefixes []string) string {
	if len(pathPrefixes) == 0 {
		return cmd
	}
	export := "export PATH=" + strings.Join(pathPrefixes, ":") + ":$PATH"
	return export + " && " + cmd
}

func (r *Runner) SetupToolchain(ctx context.Context) (ToolchainPlan, error) {
	if r.Out != nil {
		r.Out.Step("Syncing repository...")
	}
	reason, err := r.syncIfNeeded(ctx, false)
	if err != nil {
		return ToolchainPlan{}, err
	}
	if reason != SyncSkippedNone {
		r.logSyncSkip(reason)
	}
	plan, err := DetectPlan(r.Cwd, r.Proj, "")
	if err != nil {
		return ToolchainPlan{}, err
	}
	if r.Proj.EnvironmentEnabled() {
		manager := environment.New(r.Exec, r.Proj, r.Cwd)
		if err := manager.Ensure(ctx); err != nil {
			return ToolchainPlan{}, err
		}
		if len(plan.Packages) > 0 {
			packages := strings.Join(plan.Packages, " ")
			install := fmt.Sprintf("if command -v apt-get >/dev/null 2>&1; then apt-get update -qq && apt-get install -y -qq %s; elif command -v apk >/dev/null 2>&1; then apk add --no-cache %s; else echo 'no supported package manager in development image' >&2; exit 1; fi", packages, packages)
			code, err := manager.ExecCommand(ctx, install, transport.RunOpts{Stdout: os.Stdout, Stderr: os.Stderr})
			if err != nil {
				return ToolchainPlan{}, err
			}
			if code != 0 {
				return ToolchainPlan{}, fmt.Errorf("could not install toolchain packages in development container")
			}
		}
		return plan, nil
	}
	if _, err := r.ensureToolchainWithCache(ctx, plan, r.Out); err != nil {
		return ToolchainPlan{}, err
	}
	return plan, nil
}

func (r *Runner) ensureToolchainForRun(ctx context.Context, command string, skip bool) (string, error) {
	if skip || !r.Proj.ToolchainAuto() {
		return command, nil
	}
	plan, err := DetectPlan(r.Cwd, r.Proj, command)
	if err != nil {
		return "", err
	}
	if plan.Empty() {
		return command, nil
	}
	pathPrefixes, err := r.ensureToolchainWithCache(ctx, plan, r.Out)
	if err != nil {
		return "", err
	}
	return wrapCommandPath(command, pathPrefixes), nil
}
