package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/goke/outpost/internal/authz"
	"github.com/goke/outpost/internal/bootstrap"
	"github.com/goke/outpost/internal/compose"
	"github.com/goke/outpost/internal/config"
	"github.com/goke/outpost/internal/connect"
	"github.com/goke/outpost/internal/docker"
	"github.com/goke/outpost/internal/host"
	"github.com/goke/outpost/internal/output"
	"github.com/goke/outpost/internal/project"
	"github.com/goke/outpost/internal/share"
	"github.com/goke/outpost/internal/transport"
	"github.com/spf13/cobra"
)

type App struct {
	Global   *config.Global
	Out      *output.Printer
	HostFlag string
	Cwd      string
	ForceYes bool
}

func New() *cobra.Command {
	app := &App{
		Out: output.New(false, false),
		Cwd: mustCwd(),
	}
	root := &cobra.Command{
		Use:   "outpost",
		Short: "Turn a remote Linux host into a shared development runtime",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			g, err := config.LoadGlobal()
			if err != nil {
				return err
			}
			app.Global = g
			jsonOut, _ := cmd.Flags().GetBool("json")
			debug, _ := cmd.Flags().GetBool("debug")
			app.Out = output.New(jsonOut, debug)
			app.HostFlag, _ = cmd.Flags().GetString("host")
			app.ForceYes, _ = cmd.Flags().GetBool("yes")
			return nil
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().Bool("json", false, "machine-readable JSON output")
	root.PersistentFlags().Bool("debug", false, "debug logging")
	root.PersistentFlags().String("host", "", "target host name")
	root.PersistentFlags().Bool("yes", false, "skip confirmation prompts")

	root.AddCommand(app.hostCmd())
	root.AddCommand(app.initCmd())
	root.AddCommand(app.dockerCmd())
	root.AddCommand(app.composeCmd())
	root.AddCommand(app.connectCmd())
	root.AddCommand(app.inviteCmd())

	return root
}

func mustCwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

func (app *App) resolveHostName(proj *config.Project) string {
	if app.HostFlag != "" {
		return app.HostFlag
	}
	if proj != nil && proj.Host != "" {
		return proj.Host
	}
	return app.Global.ActiveHost
}

func (app *App) withExecutor(run func(context.Context, transport.Executor, *config.Host) error) error {
	exec, h, err := host.NewExecutor(app.Global, app.HostFlag)
	if err != nil {
		return err
	}
	if c, ok := exec.(*transport.SSHExecutor); ok {
		defer c.Close()
	}
	ctx := context.Background()
	if err := bootstrap.Ensure(ctx, exec); err != nil {
		return err
	}
	if err := authz.RequireRuntimeAccess(ctx, h, exec); err != nil {
		return err
	}
	return run(ctx, exec, h)
}

func (app *App) withProjectExecutor(run func(context.Context, transport.Executor, *config.Host, *config.Project) error) error {
	proj, err := config.LoadProject(app.Cwd)
	if err != nil {
		return err
	}
	hostName := app.resolveHostName(proj)
	exec, h, err := host.NewExecutor(app.Global, hostName)
	if err != nil {
		return err
	}
	if c, ok := exec.(*transport.SSHExecutor); ok {
		defer c.Close()
	}
	ctx := context.Background()
	if err := bootstrap.Ensure(ctx, exec); err != nil {
		return err
	}
	if err := authz.RequireRuntimeAccess(ctx, h, exec); err != nil {
		return err
	}
	return run(ctx, exec, h, proj)
}

func (app *App) hostCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "host", Short: "Manage remote hosts"}
	cmd.AddCommand(app.hostAddCmd())
	cmd.AddCommand(app.hostListCmd())
	cmd.AddCommand(app.hostUseCmd())
	cmd.AddCommand(app.hostVerifyCmd())
	cmd.AddCommand(app.hostRemoveCmd())
	cmd.AddCommand(app.hostDestroyCmd())
	return cmd
}

func (app *App) hostAddCmd() *cobra.Command {
	var hostname, user, identityFile string
	var port int
	cmd := &cobra.Command{
		Use:   "add NAME",
		Short: "Register a remote host",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := &host.Service{Global: app.Global, Out: app.Out}
			if identityFile == "" {
				home, _ := os.UserHomeDir()
				identityFile = filepath.Join(home, ".ssh", "id_ed25519")
			}
			if err := svc.Add(args[0], hostname, user, port, identityFile); err != nil {
				return err
			}
			app.Out.Success("Host %q registered", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&hostname, "hostname", "", "SSH hostname or IP")
	cmd.Flags().StringVar(&user, "user", "", "SSH user")
	cmd.Flags().IntVar(&port, "port", 22, "SSH port")
	cmd.Flags().StringVar(&identityFile, "identity-file", "", "path to SSH private key")
	_ = cmd.MarkFlagRequired("hostname")
	_ = cmd.MarkFlagRequired("user")
	return cmd
}

func (app *App) hostListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List registered hosts",
		RunE: func(cmd *cobra.Command, args []string) error {
			return (&host.Service{Global: app.Global, Out: app.Out}).List()
		},
	}
}

func (app *App) hostUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use NAME",
		Short: "Set the active host",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := (&host.Service{Global: app.Global, Out: app.Out}).Use(args[0]); err != nil {
				return err
			}
			app.Out.Success("Active host: %s", args[0])
			return nil
		},
	}
}

func (app *App) hostVerifyCmd() *cobra.Command {
	var skipBootstrap bool
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify connectivity and bootstrap remote dependencies",
		RunE: func(cmd *cobra.Command, args []string) error {
			return (&host.Service{Global: app.Global, Out: app.Out}).Verify(context.Background(), app.HostFlag, skipBootstrap)
		},
	}
	cmd.Flags().BoolVar(&skipBootstrap, "skip-bootstrap", false, "only test SSH connectivity")
	return cmd
}

func (app *App) hostRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove NAME",
		Short: "Remove local host configuration (does not affect the remote server)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := (&host.Service{Global: app.Global, Out: app.Out}).Remove(args[0]); err != nil {
				return err
			}
			app.Out.Success("Removed local configuration for host %q", args[0])
			return nil
		},
	}
}

func (app *App) hostDestroyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "destroy NAME",
		Short: "Destroy a cloud host (owner only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			h, err := app.Global.ResolveHost(args[0])
			if err != nil {
				return err
			}
			if err := authz.RequireOwner(h, "host destroy"); err != nil {
				return err
			}
			return authz.DenyProviderAndDestroy("host destroy")
		},
	}
}

func (app *App) initCmd() *cobra.Command {
	var name, hostName string
	var writeGitignore bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize Outpost for the current repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := project.Init(app.Cwd, name, hostName, writeGitignore)
			if err != nil {
				return err
			}
			if app.Out.JSON {
				return app.Out.PrintJSON(p)
			}
			app.Out.Success("Initialized project %q", p.Name)
			app.Out.Info("Remote directory: %s", p.RemoteDir)
			if !writeGitignore {
				app.Out.Info("Tip: add .outpost/ to .gitignore or re-run with --write-gitignore")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "stable project name")
	cmd.Flags().StringVar(&hostName, "host", "", "host override")
	cmd.Flags().BoolVar(&writeGitignore, "write-gitignore", false, "append .outpost/ to .gitignore")
	return cmd
}

func (app *App) dockerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "docker [args...]",
		Short:              "Run docker on the remote host",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.withExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host) error {
				code, err := docker.Run(ctx, exec, args)
				if err != nil {
					return err
				}
				if code != 0 {
					os.Exit(code)
				}
				return nil
			})
		},
	}
	return cmd
}

func (app *App) composeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compose",
		Short: "Run docker compose on the remote host",
	}
	cmd.AddCommand(app.composeSubCmd("up", true, false))
	cmd.AddCommand(app.composeSubCmd("down", false, true))
	cmd.AddCommand(app.composeSubCmd("ps", false, false))
	cmd.AddCommand(app.composeSubCmd("logs", false, false))
	cmd.AddCommand(app.composeSubCmd("exec", false, false))
	cmd.AddCommand(app.composeSubCmd("build", true, false))
	cmd.AddCommand(app.composeSubCmd("pull", true, false))
	return cmd
}

func (app *App) composeSubCmd(sub string, uploadFirst, checkDestructive bool) *cobra.Command {
	return &cobra.Command{
		Use:                sub,
		Short:              fmt.Sprintf("docker compose %s on the remote host", sub),
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.withProjectExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project) error {
				if checkDestructive && compose.IsDestructive(sub, args) {
					count, _ := share.ApprovedCount(ctx, exec)
					if err := authz.ConfirmDestructive(count, "compose "+sub, app.ForceYes); err != nil {
						return err
					}
				}
				runner := &compose.Runner{Exec: exec, Project: proj, Cwd: app.Cwd}
				code, err := runner.Run(ctx, sub, args, uploadFirst)
				if err != nil {
					return err
				}
				if code != 0 {
					os.Exit(code)
				}
				return nil
			})
		},
	}
}

func (app *App) connectCmd() *cobra.Command {
	var service string
	var status, down bool
	var localPort int
	var portSpecs []string
	cmd := &cobra.Command{
		Use:   "connect",
		Short: "Forward Compose service ports to localhost",
		RunE: func(cmd *cobra.Command, args []string) error {
			if status {
				return app.connectStatus()
			}
			if down {
				return app.connectDown()
			}
			return app.connectStart(service, localPort, portSpecs)
		},
	}
	cmd.Flags().StringVar(&service, "service", "", "forward ports for a single service")
	cmd.Flags().BoolVar(&status, "status", false, "show active forwarding sessions")
	cmd.Flags().BoolVar(&down, "down", false, "stop forwarding sessions")
	cmd.Flags().IntVar(&localPort, "local-port", 0, "override local port for single-service mode")
	cmd.Flags().StringArrayVar(&portSpecs, "port", nil, "manual port mapping (e.g. 8080:80)")
	return cmd
}

func (app *App) connectStart(service string, localPortOverride int, portSpecs []string) error {
	return app.withProjectExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project) error {
		mappings, err := connect.ParseComposePorts(app.Cwd, proj, service)
		if err != nil && len(portSpecs) == 0 {
			return err
		}
		for _, spec := range portSpecs {
			pm, err := connect.ParseManualPort(spec)
			if err != nil {
				return err
			}
			mappings = connect.MergePortMappings(mappings, []connect.PortMapping{pm})
		}
		if len(mappings) == 0 {
			return fmt.Errorf("no ports to forward — publish ports in compose or pass --port")
		}
		overrides := map[string]int{}
		if localPortOverride > 0 && len(mappings) == 1 {
			overrides[mappings[0].Service] = localPortOverride
		}
		forwards, closers, err := connect.StartForwards(ctx, exec, mappings, overrides)
		if err != nil {
			return err
		}
		sess := &connect.Session{
			Host:      h.Name,
			Project:   proj.Name,
			PID:       os.Getpid(),
			StartedAt: time.Now().UTC(),
			Forwards:  forwards,
		}
		if err := connect.SaveSession(sess); err != nil {
			for _, c := range closers {
				c.Close()
			}
			return err
		}
		for _, f := range forwards {
			app.Out.Success("%s (service: %s)", f.URL, f.Service)
		}
		app.Out.Info("Press Ctrl+C to stop forwarding")
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		for _, c := range closers {
			c.Close()
		}
		_ = connect.RemoveSession(h.Name, proj.Name)
		return nil
	})
}

func (app *App) connectStatus() error {
	proj, err := config.LoadProject(app.Cwd)
	if err != nil {
		return err
	}
	hostName := app.resolveHostName(proj)
	sess, err := connect.LoadSession(hostName, proj.Name)
	if err != nil {
		app.Out.Info("No active forwarding session")
		return nil
	}
	if app.Out.JSON {
		return app.Out.PrintJSON(sess)
	}
	for _, f := range sess.Forwards {
		app.Out.Info("%s (service: %s)", f.URL, f.Service)
	}
	return nil
}

func (app *App) connectDown() error {
	proj, err := config.LoadProject(app.Cwd)
	if err != nil {
		return err
	}
	hostName := app.resolveHostName(proj)
	if err := connect.StopSession(hostName, proj.Name); err != nil {
		return err
	}
	app.Out.Success("Forwarding session stopped")
	return nil
}

func (app *App) inviteCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "invite", Short: "Manage host sharing invitations"}
	cmd.AddCommand(app.inviteCreateCmd())
	cmd.AddCommand(app.inviteJoinCmd())
	cmd.AddCommand(app.inviteListCmd())
	cmd.AddCommand(app.inviteApproveCmd())
	cmd.AddCommand(app.inviteRevokeCmd())
	return cmd
}

func (app *App) inviteCreateCmd() *cobra.Command {
	var ttl string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an invitation code",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := time.ParseDuration(ttl)
			if err != nil {
				return fmt.Errorf("invalid --ttl: %w", err)
			}
			return app.withExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host) error {
				if err := authz.RequireOwner(h, "invite create"); err != nil {
					return err
				}
				svc := &share.Service{Global: app.Global, Exec: exec, Host: h}
				code, err := svc.CreateInvitation(ctx, d)
				if err != nil {
					return err
				}
				if app.Out.JSON {
					return app.Out.PrintJSON(map[string]string{"code": code})
				}
				app.Out.Success("Invitation code: %s", code)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&ttl, "ttl", "72h", "invitation lifetime")
	return cmd
}

func (app *App) inviteJoinCmd() *cobra.Command {
	var label, hostname, user, identityFile, joinHost string
	var port int
	cmd := &cobra.Command{
		Use:   "join CODE",
		Short: "Join a host using an invitation code",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if label == "" {
				label, _ = os.Hostname()
			}
			return app.inviteJoin(args[0], label, joinHost, hostname, user, port, identityFile)
		},
	}
	cmd.Flags().StringVar(&label, "label", "", "device label")
	cmd.Flags().StringVar(&joinHost, "host", "", "registered host used to reach the server for registration")
	cmd.Flags().StringVar(&hostname, "hostname", "", "remote hostname (required when --host is not set)")
	cmd.Flags().StringVar(&user, "user", "", "SSH user (required when --host is not set)")
	cmd.Flags().IntVar(&port, "port", 22, "SSH port")
	cmd.Flags().StringVar(&identityFile, "identity-file", "", "SSH identity file for registration")
	return cmd
}

func (app *App) inviteJoin(code, label, joinHost, hostname, user string, port int, identityFile string) error {
	ctx := context.Background()
	var exec transport.Executor
	var h *config.Host
	var close func()

	if joinHost != "" {
		var err error
		exec, h, err = host.NewExecutor(app.Global, joinHost)
		if err != nil {
			return err
		}
		if c, ok := exec.(*transport.SSHExecutor); ok {
			close = func() { c.Close() }
		}
		if hostname == "" {
			hostname = h.Hostname
		}
		if user == "" {
			user = h.User
		}
	} else {
		if hostname == "" || user == "" {
			return fmt.Errorf("join requires --hostname and --user, or an existing --host entry for registration SSH")
		}
		if identityFile == "" {
			home, _ := os.UserHomeDir()
			identityFile = filepath.Join(home, ".ssh", "id_ed25519")
		}
		var err error
		sshExec, err := transport.NewSSH(transport.SSHConfig{
			Hostname:     hostname,
			User:         user,
			Port:         port,
			IdentityFile: config.ExpandPath(identityFile),
		})
		if err != nil {
			return err
		}
		exec = sshExec
		close = func() { sshExec.Close() }
		h = &config.Host{Name: "join", Hostname: hostname, User: user, Port: port, Role: config.RoleMember}
	}
	if close != nil {
		defer close()
	}
	svc := &share.Service{Global: app.Global, Exec: exec, Host: h}
	if err := svc.JoinInvitation(ctx, code, label, hostname, user, port); err != nil {
		return err
	}
	app.Out.Success("Join request submitted for device %q (pending owner approval)", label)
	return nil
}

func (app *App) inviteListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List invitations and devices",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.withExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host) error {
				if err := authz.RequireOwner(h, "invite list"); err != nil {
					return err
				}
				svc := &share.Service{Global: app.Global, Exec: exec, Host: h}
				m, err := svc.List(ctx)
				if err != nil {
					return err
				}
				if app.Out.JSON {
					return app.Out.PrintJSON(m)
				}
				for _, inv := range m.Invitations {
					app.Out.Info("invitation %s expires %s", inv.Code, inv.ExpiresAt.Format(time.RFC3339))
				}
				for _, d := range m.Devices {
					app.Out.Info("device %s %s status=%s", d.ID[:8], d.Label, d.Status)
				}
				return nil
			})
		},
	}
}

func (app *App) inviteApproveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "approve DEVICE_ID",
		Short: "Approve a pending device",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.withExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host) error {
				if err := authz.RequireOwner(h, "invite approve"); err != nil {
					return err
				}
				svc := &share.Service{Global: app.Global, Exec: exec, Host: h}
				if err := svc.Approve(ctx, args[0]); err != nil {
					return err
				}
				app.Out.Success("Device approved")
				return nil
			})
		},
	}
}

func (app *App) inviteRevokeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "revoke DEVICE_ID",
		Short: "Revoke device access",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.withExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host) error {
				if err := authz.RequireOwner(h, "invite revoke"); err != nil {
					return err
				}
				svc := &share.Service{Global: app.Global, Exec: exec, Host: h}
				if err := svc.Revoke(ctx, args[0]); err != nil {
					return err
				}
				app.Out.Success("Device revoked")
				return nil
			})
		},
	}
}
