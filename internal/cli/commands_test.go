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
	root.SetArgs([]string{"init", "--name", "myapp", "--no-shell"})
	require.NoError(t, root.Execute())

	data, err := os.ReadFile(filepath.Join(cwd, ".outpost", "project.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(data), "name: myapp")
}

func TestSimplifiedSurfaceOmitsRemovedGroups(t *testing.T) {
	root, _ := cli.NewWithApp()
	commands := map[string]bool{}
	for _, command := range root.Commands() {
		commands[command.Name()] = true
	}
	for _, removed := range []string{"mirror", "connect", "up", "down", "logs"} {
		require.False(t, commands[removed], "removed command %q is still public", removed)
	}
	for _, current := range []string{"app", "compose", "cluster", "machine", "open", "close", "ai"} {
		require.True(t, commands[current], "current command %q is missing", current)
	}
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
		{name: "use", args: []string{"use", "dev"}},
		{name: "host capabilities", args: []string{"host", "capabilities"}},
		{name: "status", args: []string{"status"}},
		{name: "top", args: []string{"top"}},
		{name: "capacity", args: []string{"capacity"}},
		{name: "disk", args: []string{"disk"}},
		{name: "docker ps", args: []string{"docker", "ps"}},
		{name: "compose ps", args: []string{"compose", "ps"}},
		{name: "compose volumes list", args: []string{"compose", "volumes", "list"}},
		{name: "close no session", args: []string{"close"}, wantErr: true},
		{name: "migrate dry-run", args: []string{"migrate", "--to", "staging", "--dry-run", "--yes"}},
		{name: "invite list", args: []string{"invite", "list"}},
		{name: "invite create", args: []string{"invite", "create"}},
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
	"host add":                "requires real SSH to verify the host",
	"host verify":             "opens a real SSH connection",
	"host start":              "requires AWS EC2 API",
	"host stop":               "requires AWS EC2 API",
	"host restart":            "requires AWS EC2 API",
	"host resize":             "requires AWS EC2 API",
	"host update-ssh-access":  "requires AWS EC2 API",
	"top --watch":             "blocks until interrupted",
	"compose up":              "upload + capacity checks + long-running deploy",
	"compose down":            "destructive without dry-run guard in smoke tests",
	"compose build":           "upload + remote image build",
	"compose pull":            "upload + remote image pull",
	"compose logs":            "may stream indefinitely",
	"compose exec":            "interactive session",
	"app build":               "builds the project Dockerfile image",
	"app run":                 "runs the project Dockerfile application",
	"app stop":                "stops the project application",
	"app logs":                "may stream indefinitely",
	"app status":              "requires an existing application container",
	"cluster up":              "provisions the project Kubernetes container and cluster",
	"cluster down":            "destructive project Kubernetes cluster removal",
	"cluster env":             "runs an arbitrary local command with a live project tunnel",
	"cluster status":          "requires a project and remote Kubernetes status",
	"machine up":              "provisions the project Incus machine",
	"machine down":            "destructive project machine removal",
	"machine status":          "requires a project machine",
	"machine shell":           "interactive project machine shell",
	"machine exec":            "runs a command in the project machine",
	"machine copy":            "copies files to or from the project machine",
	"machine connect":         "forwards ports from the project machine",
	"machine snapshot create": "creates a project machine snapshot",
	"machine snapshot list":   "lists project machine snapshots",
	"machine snapshot delete": "destructive snapshot removal",
	"compose volumes export":  "creates local archives from remote volumes",
	"compose volumes import":  "restores remote volumes from archives",
	"invite join":             "requires SSH registration unless fully mocked join flow",
	"invite approve":          "requires pending device in manifest",
	"invite revoke":           "requires existing device id",
	"prune":                   "destructive without --dry-run",
	"prune volumes":           "destructive volume removal",
	"prune clusters":          "destructive cluster removal",
	"prune machines":          "destructive machine removal",
	"shell":                   "interactive remote development container",
	"ai":                      "interactive remote AI agent session",
	"run":                     "executes a command in the remote development container",
	"open":                    "starts background port forwarding",
	"cleanup":                 "requires remote host cleanup execution",
	"migrate":                 "migrates project environment between hosts",
}

func TestCLICommandCoverage(t *testing.T) {
	root, _ := cli.NewWithApp()
	tested := map[string]bool{
		"host list":            true,
		"host use":             true,
		"use":                  true,
		"host remove":          true,
		"host capabilities":    true,
		"host create":          true, // error-path smoke test
		"host destroy":         true,
		"provider login":       true,
		"init":                 true,
		"reset":                true,
		"status":               true,
		"top":                  true,
		"capacity":             true,
		"disk":                 true,
		"docker":               true, // exercised via docker ps
		"compose ps":           true,
		"compose volumes list": true,
		"invite list":          true,
		"invite create":        true,
		"invite join":          true, // error-path smoke test
		"prune":                true, // --dry-run
		"close":                true, // error-path smoke test
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

func TestOpenCommandExposesPortFlags(t *testing.T) {
	root, _ := cli.NewWithApp()
	open := findSubcommand(root, "open")
	require.NotNil(t, open)
	require.NotNil(t, open.Flags().Lookup("port"))
	require.NotNil(t, open.Flags().Lookup("local-port"))
	require.NotNil(t, open.Flags().Lookup("service"))
}

func TestAICommandExposesFlags(t *testing.T) {
	root, _ := cli.NewWithApp()
	ai := findSubcommand(root, "ai")
	require.NotNil(t, ai)
	require.NotNil(t, ai.Flags().Lookup("command"))
	require.NotNil(t, ai.Flags().Lookup("no-pull"))
}

func findSubcommand(cmd *cobra.Command, name string) *cobra.Command {
	for _, sub := range cmd.Commands() {
		if sub.Name() == name {
			return sub
		}
		if found := findSubcommand(sub, name); found != nil {
			return found
		}
	}
	return nil
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
