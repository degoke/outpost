package mock

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/degoke/outpost/internal/transport"
)

type Executor struct {
	mu        sync.Mutex
	Commands  []string
	Uploads   map[string][]byte
	Files     map[string][]byte
	Responses map[string]struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}
	HostInfoStr string
	ForwardErr  error
}

func New() *Executor {
	return &Executor{
		Uploads: map[string][]byte{},
		Files:   map[string][]byte{},
		Responses: map[string]struct {
			Stdout   string
			Stderr   string
			ExitCode int
			Err      error
		}{},
		HostInfoStr: "mock@localhost:22",
	}
}

func (m *Executor) HostInfo() string {
	if m.HostInfoStr != "" {
		return m.HostInfoStr
	}
	return "mock@localhost:22"
}

func (m *Executor) Run(ctx context.Context, cmd string, opts transport.RunOpts) (int, error) {
	if opts.WorkDir != "" {
		cmd = applyWorkDir(cmd, opts.WorkDir)
	}
	m.mu.Lock()
	m.Commands = append(m.Commands, cmd)
	resp, ok := m.matchResponse(cmd)
	m.mu.Unlock()
	if !ok {
		return 0, nil
	}
	if resp.Stdout != "" && opts.Stdout != nil {
		io.WriteString(opts.Stdout, resp.Stdout)
	}
	if resp.Stderr != "" && opts.Stderr != nil {
		io.WriteString(opts.Stderr, resp.Stderr)
	}
	return resp.ExitCode, resp.Err
}

func (m *Executor) RunInteractive(ctx context.Context, cmd string, opts transport.RunOpts) error {
	code, err := m.Run(ctx, cmd, opts)
	if err != nil {
		return err
	}
	if code != 0 {
		return &transport.ExitError{Code: code}
	}
	return nil
}

func (m *Executor) Upload(local, remote string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Uploads[remote] = []byte("uploaded:" + local)
	return nil
}

func (m *Executor) UploadBytes(data []byte, remote string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Uploads[remote] = append([]byte(nil), data...)
	return nil
}

func (m *Executor) Download(remote string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if data, ok := m.Files[remote]; ok {
		return data, nil
	}
	if data, ok := m.Uploads[remote]; ok {
		return data, nil
	}
	return nil, fmt.Errorf("file not found: %s", remote)
}

func (m *Executor) RunMkdir(dir string) error {
	m.mu.Lock()
	m.Commands = append(m.Commands, "mkdir -p "+dir)
	m.mu.Unlock()
	return nil
}

func (m *Executor) Forward(ctx context.Context, spec transport.ForwardSpec) (io.Closer, error) {
	if m.ForwardErr != nil {
		return nil, m.ForwardErr
	}
	return nopCloser{}, nil
}

type nopCloser struct{}

func (nopCloser) Close() error { return nil }

func (m *Executor) matchResponse(cmd string) (struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}, bool) {
	if r, ok := m.Responses[cmd]; ok {
		return r, true
	}
	for prefix, r := range m.Responses {
		if strings.HasPrefix(cmd, prefix) {
			return r, true
		}
	}
	return struct {
		Stdout   string
		Stderr   string
		ExitCode int
		Err      error
	}{}, false
}

func (m *Executor) LastCommand() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.Commands) == 0 {
		return ""
	}
	return m.Commands[len(m.Commands)-1]
}

func (m *Executor) HasCommand(substr string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.Commands {
		if strings.Contains(c, substr) {
			return true
		}
	}
	return false
}

func applyWorkDir(cmd, workDir string) string {
	if workDir == "" {
		return cmd
	}
	escaped := strings.ReplaceAll(workDir, "'", "'\\''")
	return fmt.Sprintf("cd '%s' && %s", escaped, cmd)
}
