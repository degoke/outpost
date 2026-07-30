package upload

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/degoke/outpost/internal/config"
	"github.com/degoke/outpost/internal/transport"
)

func syncRepoRsync(cwd string, proj *config.Project, ssh *transport.SSHExecutor, opts *SyncOpts) error {
	if _, err := exec.LookPath("rsync"); err != nil {
		return fmt.Errorf("rsync not found locally — install rsync or sync without --rsync")
	}
	cfg := ssh.Config()
	if cfg.AuthMode == transport.AuthPassword && cfg.IdentityFile == "" {
		return fmt.Errorf("rsync sync requires SSH key authentication (password-only auth is not supported)")
	}

	ctx := context.Background()
	code, err := ssh.Run(ctx, "command -v rsync >/dev/null 2>&1", transport.RunOpts{})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("rsync not found on remote host — install rsync or sync without --rsync")
	}
	if err := transport.EnsureRemoteDir(ssh, proj.RemoteDir); err != nil {
		return err
	}

	if opts != nil && opts.Out != nil && !opts.Out.JSON {
		opts.Out.Step("Syncing repository with rsync...")
	}

	sshArgs := strings.Join(quoteSSHArgs(ssh.RsyncSSHArgs()), " ")
	remoteDest := fmt.Sprintf("%s:%s/", ssh.Destination(), proj.RemoteDir)

	args := []string{
		"-az",
		"--delete",
	}
	for _, pattern := range alwaysIgnore {
		args = append(args, "--exclude", pattern+"/", "--exclude", pattern)
	}
	if isGitRepo(cwd) {
		args = append(args, "--filter", ":- .gitignore")
	}
	if HasOutpostIgnoreFile(cwd) {
		if excludeFrom, ok := OutpostIgnoreExcludeArg(cwd); ok {
			args = append(args, "--exclude-from", excludeFrom)
		}
	}
	args = append(args, "-e", "ssh "+sshArgs, cwd+"/", remoteDest)

	cmd := exec.Command("rsync", args...)
	cmd.Dir = cwd
	if opts != nil && opts.Out != nil && !opts.Out.JSON {
		cmd.Stdout = opts.Out.Stderr
		cmd.Stderr = opts.Out.Stderr
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rsync: %w", err)
	}
	return nil
}

func quoteSSHArgs(args []string) []string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		if strings.ContainsAny(arg, " \t") {
			quoted[i] = "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
		} else {
			quoted[i] = arg
		}
	}
	return quoted
}
