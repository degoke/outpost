package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/mirror"
	"github.com/degoke/outpost/internal/transport"
	"github.com/spf13/cobra"
)

func (app *App) sessionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Inspect and reconnect to persistent outpost run sessions",
	}
	cmd.AddCommand(app.sessionListCmd())
	cmd.AddCommand(app.sessionStatusCmd())
	cmd.AddCommand(app.sessionAttachCmd())
	cmd.AddCommand(app.sessionLogsCmd())
	cmd.AddCommand(app.sessionKillCmd())
	return cmd
}

func (app *App) sessionListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List run sessions for this project (running and recently finished)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.withProjectExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project) error {
				runner := mirror.New(exec, proj, app.Cwd, h.Name, app.Out)
				sessions, err := runner.ListSessions(ctx)
				if err != nil {
					return err
				}
				if app.Out.JSON {
					return app.Out.PrintJSON(sessions)
				}
				if len(sessions) == 0 {
					app.Out.Info("No sessions for project %q", proj.Name)
					return nil
				}
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "NAME\tSTATE\tEXIT\tSTARTED\tCOMMAND")
				for _, s := range sessions {
					state := "running"
					exit := "-"
					if !s.Running {
						state = "finished"
					}
					if s.ExitCode != nil {
						exit = fmt.Sprintf("%d", *s.ExitCode)
					}
					started := "-"
					if !s.StartedAt.IsZero() {
						started = s.StartedAt.UTC().Format(time.RFC3339)
					}
					command := strings.TrimSpace(s.Command)
					if len(command) > 60 {
						command = command[:57] + "..."
					}
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", s.Name, state, exit, started, command)
				}
				return w.Flush()
			})
		},
	}
}

func (app *App) sessionStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status NAME",
		Short: "Show status and recent logs for a run session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.withProjectExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project) error {
				runner := mirror.New(exec, proj, app.Cwd, h.Name, app.Out)
				status, err := runner.SessionStatus(ctx, args[0])
				if err != nil {
					return err
				}
				if app.Out.JSON {
					return app.Out.PrintJSON(status)
				}
				state := "running"
				if !status.Running {
					state = "finished"
				}
				app.Out.Info("Session %q is %s", status.Name, state)
				if !status.StartedAt.IsZero() {
					app.Out.Info("Started: %s", status.StartedAt.UTC().Format(time.RFC3339))
				}
				if status.Command != "" {
					app.Out.Info("Command: %s", status.Command)
				}
				if status.ExitCode != nil {
					app.Out.Info("Exit code: %d", *status.ExitCode)
				}
				if status.LogTail != "" {
					fmt.Fprintf(os.Stdout, "\n%s\n", status.LogTail)
				}
				return nil
			})
		},
	}
}

func (app *App) sessionAttachCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "attach NAME",
		Short: "Attach to a running run session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.withProjectExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project) error {
				result, err := mirror.New(exec, proj, app.Cwd, h.Name, app.Out).Run(ctx, mirror.RunOptions{
					AttachSession: args[0],
				})
				return exitRunResult(app, result, err)
			})
		},
	}
}

func (app *App) sessionLogsCmd() *cobra.Command {
	var follow bool
	var lines int
	cmd := &cobra.Command{
		Use:   "logs NAME",
		Short: "Show logs for a run session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.withProjectExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project) error {
				return mirror.New(exec, proj, app.Cwd, h.Name, app.Out).SessionLogs(ctx, args[0], follow, lines)
			})
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow log output")
	cmd.Flags().IntVar(&lines, "tail", 50, "number of lines to show")
	return cmd
}

func (app *App) sessionKillCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "kill NAME",
		Short: "Stop a running run session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.withProjectExecutor(func(ctx context.Context, exec transport.Executor, h *config.Host, proj *config.Project) error {
				if err := mirror.New(exec, proj, app.Cwd, h.Name, app.Out).KillSession(ctx, args[0]); err != nil {
					return err
				}
				app.Out.Success("Stopped session %q", args[0])
				return nil
			})
		},
	}
}
