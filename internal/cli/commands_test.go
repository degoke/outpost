package cli_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/degoke/outpost/internal/cli"
	"github.com/degoke/outpost/internal/testenv"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

type cliCase struct {
	name    string
	args    []string
	wantErr bool
	setup   func(t *testing.T, e *cliEnv)
}

func TestCLIInit(t *testing.T) {
	cwd := t.TempDir()
	compose := `services:
  web:
    image: nginx:alpine
`
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "docker-compose.yml"), []byte(compose), 0o644))

	home := t.TempDir()
	t.Setenv("HOME", home)
	testenv.UseHomeConfigDir(t, home)
	writeTestGlobal(t, home)

	root, app := cli.NewWithApp()
	app.Cwd = cwd
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"init", "--name", "myapp"})
	require.NoError(t, root.Execute())

	data, err := os.ReadFile(filepath.Join(cwd, ".outpost", "project.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(data), "name: myapp")
}

func TestCLIReset(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	testenv.UseHomeConfigDir(t, home)
	writeTestGlobal(t, home)

	root, _ := cli.NewWithApp()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"reset", "--yes"})
	require.NoError(t, root.Execute())

	_, err := os.Stat(filepath.Join(home, ".outpost", "config.yaml"))
	require.True(t, os.IsNotExist(err))
}

func TestCLIHostUseAndRemove(t *testing.T) {
	e := newCLIEnv(t)
	require.NoError(t, e.run("host", "use", "dev"))

	configPath := filepath.Join(e.home, ".outpost", "config.yaml")
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Contains(t, string(data), "active_host: dev")

	// Add a second host entry locally (no SSH) and remove it.
	dir := filepath.Join(e.home, ".outpost")
	extraHost := `version: 1
active_host: dev
hosts:
  dev:
    hostname: 203.0.113.10
    user: ubuntu
    port: 22
    role: owner
    host_id: test-host-id
  staging:
    hostname: 203.0.113.11
    user: ubuntu
    port: 22
    role: owner
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(extraHost), 0o600))
	require.NoError(t, e.run("host", "remove", "staging"))

	data, err = os.ReadFile(configPath)
	require.NoError(t, err)
	require.NotContains(t, string(data), "staging:")
}

func TestCLICommands(t *testing.T) {
	cases := []cliCase{
		{name: "host list", args: []string{"host", "list"}},
		{name: "host capabilities", args: []string{"host", "capabilities"}},
		{name: "status", args: []string{"status"}},
		{name: "top", args: []string{"top"}},
		{name: "capacity", args: []string{"capacity"}},
		{name: "disk", args: []string{"disk"}},
		{name: "docker ps", args: []string{"docker", "ps"}},
		{name: "compose ps", args: []string{"compose", "ps"}},
		{name: "compose volumes list", args: []string{"compose", "volumes", "list"}},
		{name: "mirror sync", args: []string{"mirror", "sync"}},
		{name: "mirror run no-sync", args: []string{"mirror", "run", "--no-sync", "--", "echo", "hello"}},
		{name: "mirror toolchain plan", args: []string{"mirror", "toolchain", "plan"}},
		{name: "mirror sessions list", args: []string{"mirror", "sessions", "list"}},
		{name: "connect status", args: []string{"connect", "--status"}},
		{name: "connect down no session", args: []string{"connect", "--down"}, wantErr: true},
		{name: "invite list", args: []string{"invite", "list"}},
		{name: "invite create", args: []string{"invite", "create"}},
		{name: "cluster list", args: []string{"cluster", "list"}},
		{name: "machine list", args: []string{"machine", "list"}},
		{name: "kubectl get nodes", args: []string{"kubectl", "--cluster", "dev", "get", "nodes"}},
		{name: "prune dry-run", args: []string{"prune", "--dry-run"}},

		// Commands that require cloud credentials or real SSH — expect failure, not a hang.
		{name: "host create without cloud", args: []string{"host", "create", "cloud1"}, wantErr: true},
		{name: "provider login aws", args: []string{"provider", "login", "aws"}, wantErr: true},
		{name: "host destroy non-cloud", args: []string{"host", "destroy", "dev"}, wantErr: true},
		{name: "invite join missing flags", args: []string{"invite", "join", "CODE123"}, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newCLIEnv(t)
			if tc.setup != nil {
				tc.setup(t, e)
			}
			err := e.run(tc.args...)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

// untestableCLICommands documents commands that need real SSH, AWS, or block indefinitely.
var untestableCLICommands = map[string]string{
	"host add":                 "requires real SSH to verify the host",
	"host verify":              "opens a real SSH connection",
	"host start":               "requires AWS EC2 API",
	"host stop":                "requires AWS EC2 API",
	"host restart":             "requires AWS EC2 API",
	"host resize":              "requires AWS EC2 API",
	"host update-ssh-access":   "requires AWS EC2 API",
	"mirror watch":             "blocks until interrupted",
	"mirror shell":             "interactive TTY session",
	"mirror setup-python":      "long-running remote venv bootstrap",
	"mirror setup-toolchain":   "may install apt packages and Go on remote host",
	"connect":                  "spawns a background port-forward worker by default",
	"top --watch":              "blocks until interrupted",
	"compose up":               "upload + capacity checks + long-running deploy",
	"compose down":             "destructive without dry-run guard in smoke tests",
	"compose build":            "upload + remote image build",
	"compose pull":             "upload + remote image pull",
	"compose logs":             "may stream indefinitely",
	"compose exec":             "interactive session",
	"cluster create":           "provisions Kubernetes cluster on remote host (kind or k3d)",
	"cluster delete":           "destructive cluster removal",
	"cluster status":           "requires an existing cluster name",
	"machine create":           "provisions Incus instance",
	"machine status":           "requires an existing machine name",
	"machine start":            "requires an existing machine name",
	"machine stop":             "requires an existing machine name",
	"machine restart":          "requires an existing machine name",
	"machine shell":            "interactive Incus shell",
	"machine exec":             "interactive Incus exec",
	"machine connect":          "SSH port forward to machine",
	"machine copy":             "file copy to/from machine",
	"machine snapshot create":  "requires existing machine",
	"machine snapshot list":    "requires existing machine",
	"machine snapshot restore": "requires existing snapshot",
	"machine snapshot delete":  "destructive snapshot removal",
	"machine delete":           "destructive machine removal",
	"compose volumes export":   "creates local archives from remote volumes",
	"compose volumes import":   "restores remote volumes from archives",
	"invite join":              "requires SSH registration unless fully mocked join flow",
	"invite approve":           "requires pending device in manifest",
	"invite revoke":            "requires existing device id",
	"mirror sessions status":   "requires existing session name",
	"mirror sessions logs":     "requires existing session name",
	"mirror sessions attach":   "interactive tmux attach",
	"mirror sessions kill":     "destructive session kill",
	"prune":                    "destructive without --dry-run",
	"prune volumes":            "destructive volume removal",
	"prune clusters":           "destructive cluster removal",
	"prune machines":           "destructive machine removal",
}

func TestCLICommandCoverage(t *testing.T) {
	root, _ := cli.NewWithApp()
	tested := map[string]bool{
		"host list":             true,
		"host use":              true,
		"host remove":           true,
		"host capabilities":     true,
		"host create":           true, // error-path smoke test
		"host destroy":          true,
		"provider login":        true,
		"init":                  true,
		"reset":                 true,
		"status":                true,
		"top":                   true,
		"capacity":              true,
		"disk":                  true,
		"docker":                true, // exercised via docker ps
		"compose ps":            true,
		"compose volumes list":  true,
		"mirror sync":           true,
		"mirror run":            true,
		"mirror toolchain plan": true,
		"mirror sessions list":  true,
		"connect":               true, // --status / --down variants
		"invite list":           true,
		"invite create":         true,
		"invite join":           true, // error-path smoke test
		"cluster list":          true,
		"machine list":          true,
		"kubectl":               true,
		"prune":                 true, // --dry-run
	}

	all := collectRunnableCommands(root, nil)
	var missing []string
	for path := range all {
		if tested[path] || untestableCLICommands[path] != "" {
			continue
		}
		missing = append(missing, path)
	}
	if len(missing) > 0 {
		t.Fatalf("unclassified CLI commands (add a smoke test or document in untestableCLICommands): %s", strings.Join(missing, ", "))
	}
}

func collectRunnableCommands(cmd *cobra.Command, prefix []string) map[string]bool {
	result := make(map[string]bool)
	name := cmd.Name()
	if name != "" && name != "outpost" {
		prefix = append(append([]string(nil), prefix...), name)
	}
	if len(cmd.Commands()) == 0 {
		if cmd.Runnable() {
			result[strings.Join(prefix, " ")] = true
		}
		return result
	}
	for _, sub := range cmd.Commands() {
		if sub.Hidden {
			continue
		}
		for k, v := range collectRunnableCommands(sub, prefix) {
			result[k] = v
		}
	}
	return result
}
