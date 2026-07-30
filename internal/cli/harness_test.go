package cli_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/degoke/outpost/internal/cli"
	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/project"
	"github.com/degoke/outpost/internal/testenv"
	"github.com/degoke/outpost/internal/transport"
	"github.com/degoke/outpost/internal/transport/mock"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

type mockResp struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}

func mockOK(stdout string) mockResp {
	return mockResp{Stdout: stdout}
}

func mockExit(code int, stdout string) mockResp {
	return mockResp{Stdout: stdout, ExitCode: code}
}

type cliEnv struct {
	t    *testing.T
	home string
	cwd  string
	exec *mock.Executor
	root *cobra.Command
	app  *cli.App
}

func newCLIEnv(t *testing.T) *cliEnv {
	t.Helper()
	home := t.TempDir()
	cwd := t.TempDir()
	t.Setenv("HOME", home)
	testenv.UseHomeConfigDir(t, home)

	writeTestGlobal(t, home)
	setupProject(t, cwd)

	exec := mock.New()
	seedCLIMocks(exec)

	root, app := cli.NewWithApp()
	app.Cwd = cwd
	app.SetExecutorFactory(func(g *config.Global, hostName string, autoTrustHostKey bool) (transport.Executor, *config.Host, error) {
		h, err := g.ResolveHost(hostName)
		if err != nil {
			return nil, nil, err
		}
		return exec, h, nil
	})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)

	return &cliEnv{t: t, home: home, cwd: cwd, exec: exec, root: root, app: app}
}

func (e *cliEnv) run(args ...string) error {
	e.root.SetArgs(args)
	return e.root.Execute()
}

func writeTestGlobal(t *testing.T, home string) {
	t.Helper()
	dir := filepath.Join(home, ".outpost")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	configYAML := `version: 1
active_host: dev
hosts:
  dev:
    hostname: 203.0.113.10
    user: ubuntu
    port: 22
    role: owner
    host_id: test-host-id
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(configYAML), 0o600))
}

func setupProject(t *testing.T, cwd string) {
	t.Helper()
	compose := `services:
  web:
    image: nginx:alpine
    ports:
      - "8080:80"
`
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "docker-compose.yml"), []byte(compose), 0o644))
	_, err := project.Init(cwd, "", "", false, false)
	require.NoError(t, err)
}

func seedCLIMocks(exec *mock.Executor) {
	// Bootstrap / runtime tooling
	exec.Responses["command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1"] = mockOK("")
	exec.Responses["mkdir -p /var/lib/outpost/projects /var/lib/outpost/share /var/lib/outpost/clusters /var/lib/outpost/machines && (chown -R \"$USER:$USER\" /var/lib/outpost 2>/dev/null || sudo chown -R \"$USER:$USER\" /var/lib/outpost) && test -d /var/lib/outpost/projects"] = mockOK("")
	exec.Responses["command -v free >/dev/null && command -v df >/dev/null && command -v du >/dev/null"] = mockOK("")
	exec.Responses["command -v kind >/dev/null 2>&1 && command -v kubectl >/dev/null 2>&1 && command -v k3d >/dev/null 2>&1"] = mockOK("")
	exec.Responses["command -v incus >/dev/null 2>&1 && (incus list >/dev/null 2>&1 || sudo incus list >/dev/null 2>&1)"] = mockOK("")
	exec.Responses["command -v tmux >/dev/null 2>&1"] = mockOK("")

	// Host metrics / inspect
	exec.Responses["nproc"] = mockOK("4\n")
	exec.Responses["free -b | head -2"] = mockOK("              total        used        free      shared  buff/cache   available\nMem:    8589934592  4294967296  2147483648           0  2147483648  4294967296\n")
	exec.Responses["df -B1 / | tail -1"] = mockOK("/dev/sda1 100000000000 50000000000 45000000000 53% /\n")
	exec.Responses["cat /proc/uptime"] = mockOK("3600.0 0\n")
	exec.Responses["head -1 /proc/stat"] = mockOK("cpu  100 0 50 8500 0 0 0 0 0 0\n")

	// Docker summary
	exec.Responses["docker info >/dev/null 2>&1"] = mockOK("")
	exec.Responses["docker ps"] = mockOK("")
	exec.Responses["docker ps -a"] = mockOK("")
	exec.Responses["docker images -q | wc -l"] = mockOK("0\n")
	exec.Responses["docker volume ls -q | wc -l"] = mockOK("0\n")
	exec.Responses["docker volume ls -q"] = mockOK("")
	exec.Responses["docker system df"] = mockOK("TYPE            TOTAL     ACTIVE    SIZE      RECLAIMABLE\nImages          0         0         0B        0B\n")
	exec.Responses["docker compose ls --format json"] = mockOK("")
	exec.Responses["docker stats --no-stream --format '{{json .}}'"] = mockOK("")
	exec.Responses["docker stats --no-stream --filter label=io.x-k8s.kind.role --format '{{json .}}'"] = mockOK("")
	exec.Responses["docker stats --no-stream --filter label=k3d.role --format '{{json .}}'"] = mockOK("")
	exec.Responses["docker network ls --filter dangling=true -q | wc -l"] = mockOK("0\n")

	// Clusters / machines metadata
	exec.Responses["kind get clusters 2>/dev/null || true"] = mockOK("")
	exec.Responses["k3d cluster list 2>/dev/null | awk 'NR>1 && NF {print $1}' || true"] = mockOK("")
	exec.Responses["ls -1"] = mockOK("")
	exec.Responses["incus list --format json 2>/dev/null || true"] = mockOK("[]")
	exec.Responses["docker ps --filter label=io.x-k8s.kind.cluster="] = mockOK("0\n")

	// Mirror / remote project
	exec.Responses["test -x"] = mockExit(1, "")
	exec.Responses["tmux list-sessions -F '#{session_name}' 2>/dev/null || true"] = mockOK("")

	// Remote compose / kubectl (prefix matching)
	exec.Responses["docker compose -p"] = mockOK("")
	exec.Responses["KUBECONFIG="] = mockOK("NAME   STATUS   ROLES   AGE   VERSION\n")

	// Disk / outpost dirs
	exec.Responses["du -sb /var/lib/outpost/projects /var/lib/outpost/share /var/lib/outpost/machines 2>/dev/null || true"] = mockOK("")
}
