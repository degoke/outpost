package connect

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/goke/outpost/internal/config"
	"github.com/goke/outpost/internal/transport"
)

type ResolveOptions struct {
	Service     string
	Discover    bool
	ManualSpecs []string
}

func ResolvePortMappings(ctx context.Context, exec transport.Executor, cwd string, proj *config.Project, composeArgs string, opts ResolveOptions) ([]PortMapping, error) {
	var mappings []PortMapping
	var composeErr error

	if opts.Discover {
		if exec == nil {
			return nil, fmt.Errorf("--discover requires a remote host connection")
		}
		remote, err := DiscoverRemotePorts(ctx, exec, proj.Name, composeArgs, opts.Service)
		if err != nil {
			return nil, err
		}
		mappings = remote
	} else {
		mappings, composeErr = ParseComposePorts(cwd, proj, opts.Service)
	}

	for _, spec := range opts.ManualSpecs {
		pm, err := ParseManualPort(spec)
		if err != nil {
			return nil, err
		}
		mappings = MergePortMappings(mappings, []PortMapping{pm})
	}

	if len(mappings) == 0 {
		if composeErr != nil && len(opts.ManualSpecs) == 0 {
			return nil, composeErr
		}
		return nil, fmt.Errorf("no ports to forward — publish ports in compose, pass --port, or use --discover")
	}
	return dedupeMappings(mappings), nil
}

func DiscoverRemotePorts(ctx context.Context, exec transport.Executor, projectName, composeArgs, serviceFilter string) ([]PortMapping, error) {
	cmd := fmt.Sprintf("docker compose -p %s %s ps --format json",
		shellQuote(projectName),
		strings.TrimSpace(composeArgs),
	)
	var out bytes.Buffer
	code, err := exec.Run(ctx, cmd, transport.RunOpts{Stdout: &out})
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("discover remote ports failed (exit %d)", code)
	}
	rows, err := parseComposePSJSON(out.Bytes())
	if err != nil {
		return nil, err
	}
	var mappings []PortMapping
	for _, row := range rows {
		if serviceFilter != "" && row.Service != serviceFilter {
			continue
		}
		for _, pub := range row.Publishers {
			if pub.PublishedPort < 1 {
				continue
			}
			target := pub.TargetPort
			if target < 1 {
				target = pub.PublishedPort
			}
			mappings = append(mappings, PortMapping{
				Service:    row.Service,
				HostPort:   pub.PublishedPort,
				TargetPort: target,
				BindHost:   "127.0.0.1",
			})
		}
	}
	if len(mappings) == 0 {
		return nil, fmt.Errorf("no published ports found on remote host — is the compose stack running?")
	}
	return dedupeMappings(mappings), nil
}

type composePSRow struct {
	Service    string               `json:"Service"`
	Publishers []composePSPublisher `json:"Publishers"`
}

type composePSPublisher struct {
	URL           string `json:"URL"`
	TargetPort    int    `json:"TargetPort"`
	PublishedPort int    `json:"PublishedPort"`
}

func parseComposePSJSON(data []byte) ([]composePSRow, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty docker compose ps output")
	}
	if trimmed[0] == '[' {
		var rows []composePSRow
		if err := json.Unmarshal(trimmed, &rows); err != nil {
			return nil, fmt.Errorf("parse docker compose ps json: %w", err)
		}
		return rows, nil
	}
	var rows []composePSRow
	for _, line := range bytes.Split(trimmed, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var row composePSRow
		if err := json.Unmarshal(line, &row); err != nil {
			return nil, fmt.Errorf("parse docker compose ps json: %w", err)
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no services in docker compose ps output")
	}
	return rows, nil
}

func dedupeMappings(mappings []PortMapping) []PortMapping {
	seen := map[string]bool{}
	out := make([]PortMapping, 0, len(mappings))
	for _, m := range mappings {
		key := fmt.Sprintf("%s:%d:%d", m.Service, m.HostPort, m.TargetPort)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, m)
	}
	return out
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
