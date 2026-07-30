package mirror

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/degoke/outpost/internal/output"
	"github.com/degoke/outpost/internal/transport"
)

type toolchainState struct {
	Fingerprint string `json:"fingerprint"`
}

func planFingerprint(plan ToolchainPlan) string {
	parts := append([]string{}, plan.Packages...)
	if plan.GoVersion != "" {
		parts = append(parts, "go:"+plan.GoVersion)
	}
	return strings.Join(parts, "|")
}

func pathPrefixesForPlan(plan ToolchainPlan) []string {
	if plan.GoVersion == "" {
		return nil
	}
	return []string{filepath.Join(toolchainsBase, "go", plan.GoVersion, "bin")}
}

func (r *Runner) remoteToolchainStatePath() string {
	return filepath.Join(r.Proj.RemoteDir, ".outpost", "toolchain.json")
}

func (r *Runner) readToolchainState(ctx context.Context) (toolchainState, bool) {
	path := r.remoteToolchainStatePath()
	cmd := fmt.Sprintf("cat %s 2>/dev/null", shellQuote(path))
	var stdout strings.Builder
	code, err := r.Exec.Run(ctx, cmd, transport.RunOpts{Stdout: &stdout})
	if err != nil || code != 0 {
		return toolchainState{}, false
	}
	var state toolchainState
	if err := json.Unmarshal([]byte(stdout.String()), &state); err != nil {
		return toolchainState{}, false
	}
	if strings.TrimSpace(state.Fingerprint) == "" {
		return toolchainState{}, false
	}
	return state, true
}

func (r *Runner) writeToolchainState(ctx context.Context, fingerprint string) error {
	path := r.remoteToolchainStatePath()
	dir := filepath.Dir(path)
	payload, err := json.Marshal(toolchainState{Fingerprint: fingerprint})
	if err != nil {
		return err
	}
	encoded := strings.TrimSpace(string(payload))
	script := fmt.Sprintf(
		`mkdir -p %s && printf %%s %s > %s`,
		shellQuote(dir),
		shellQuote(encoded),
		shellQuote(path),
	)
	code, err := r.Exec.Run(ctx, script, transport.RunOpts{})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("could not record remote toolchain state")
	}
	return nil
}

func (r *Runner) ensureToolchainWithCache(ctx context.Context, plan ToolchainPlan, out *output.Printer) ([]string, error) {
	fp := planFingerprint(plan)
	if state, ok := r.readToolchainState(ctx); ok && state.Fingerprint == fp {
		return pathPrefixesForPlan(plan), nil
	}
	paths, err := EnsureToolchain(ctx, r.Exec, plan, out)
	if err != nil {
		return nil, err
	}
	if err := r.writeToolchainState(ctx, fp); err != nil {
		return nil, err
	}
	return paths, nil
}
