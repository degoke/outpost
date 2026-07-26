package transport

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type SSHExecutor struct {
	cfg    SSHConfig
	client *ssh.Client
}

func NewSSH(cfg SSHConfig) (*SSHExecutor, error) {
	if cfg.Port == 0 {
		cfg.Port = 22
	}
	return &SSHExecutor{cfg: cfg}, nil
}

func (e *SSHExecutor) connect() (*ssh.Client, error) {
	if e.client != nil {
		return e.client, nil
	}
	auth, err := buildAuth(e.cfg.IdentityFile)
	if err != nil {
		return nil, err
	}
	sshCfg := &ssh.ClientConfig{
		User:            e.cfg.User,
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second,
	}
	if kh, err := knownHostsCallback(); err == nil {
		sshCfg.HostKeyCallback = kh
	}
	addr := fmt.Sprintf("%s:%d", e.cfg.Hostname, e.cfg.Port)
	client, err := ssh.Dial("tcp", addr, sshCfg)
	if err != nil {
		return nil, classifyDialError(err, e.cfg)
	}
	e.client = client
	return client, nil
}

func knownHostsCallback() (ssh.HostKeyCallback, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(home, ".ssh", "known_hosts")
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	return knownhosts.New(path)
}

func buildAuth(identityFile string) ([]ssh.AuthMethod, error) {
	if identityFile == "" {
		return nil, fmt.Errorf("no identity file configured: set --identity-file or add one with 'outpost host add'")
	}
	identityFile = expandPath(identityFile)
	key, err := os.ReadFile(identityFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("identity file not found at %s: generate a key with ssh-keygen or specify --identity-file", identityFile)
		}
		return nil, fmt.Errorf("read identity file %s: %w", identityFile, err)
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("parse identity file %s: %w (is the key encrypted? use ssh-agent or an unencrypted key)", identityFile, err)
	}
	return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
}

func classifyDialError(err error, cfg SSHConfig) error {
	if khErr, ok := err.(*knownhosts.KeyError); ok {
		return fmt.Errorf("host key for %s has changed — verify the server identity before updating ~/.ssh/known_hosts: %v", cfg.Hostname, khErr)
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "connection refused"):
		return fmt.Errorf("cannot reach %s:%d: connection refused — check that SSH is running and the port is correct", cfg.Hostname, cfg.Port)
	case strings.Contains(msg, "no route to host"), strings.Contains(msg, "network is unreachable"):
		return fmt.Errorf("cannot reach %s:%d: host unreachable — verify hostname and network connectivity", cfg.Hostname, cfg.Port)
	case strings.Contains(msg, "i/o timeout"), strings.Contains(msg, "timeout"):
		return fmt.Errorf("cannot reach %s:%d: connection timed out — check firewall rules and host availability", cfg.Hostname, cfg.Port)
	case strings.Contains(msg, "unable to authenticate"):
		return fmt.Errorf("authentication failed for %s@%s — verify identity file and authorized_keys on the host", cfg.User, cfg.Hostname)
	default:
		return fmt.Errorf("ssh connection to %s failed: %w", cfg.String(), err)
	}
}

func expandPath(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}

func (e *SSHExecutor) HostInfo() string {
	return e.cfg.String()
}

func (e *SSHExecutor) Run(ctx context.Context, cmd string, opts RunOpts) (int, error) {
	client, err := e.connect()
	if err != nil {
		return 1, err
	}
	if opts.WorkDir != "" {
		cmd = fmt.Sprintf("cd %s && %s", shellQuote(opts.WorkDir), cmd)
	}
	session, err := client.NewSession()
	if err != nil {
		return 1, err
	}
	defer session.Close()

	if opts.Stdout != nil {
		session.Stdout = opts.Stdout
	} else {
		session.Stdout = os.Stdout
	}
	if opts.Stderr != nil {
		session.Stderr = opts.Stderr
	} else {
		session.Stderr = os.Stderr
	}
	if opts.Stdin != nil {
		session.Stdin = opts.Stdin
	}

	err = session.Run(cmd)
	if err == nil {
		return 0, nil
	}
	if exitErr, ok := err.(*ssh.ExitError); ok {
		return exitErr.ExitStatus(), nil
	}
	return 1, err
}

func (e *SSHExecutor) RunInteractive(ctx context.Context, cmd string, opts RunOpts) error {
	client, err := e.connect()
	if err != nil {
		return err
	}
	if opts.WorkDir != "" {
		cmd = fmt.Sprintf("cd %s && %s", shellQuote(opts.WorkDir), cmd)
	}
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	stdin := opts.Stdin
	stdout := opts.Stdout
	stderr := opts.Stderr
	if stdin == nil {
		stdin = os.Stdin
	}
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}

	fd := int(os.Stdin.Fd())
	if isTerminal(fd) {
		if err := requestPTY(session); err != nil {
			return err
		}
	}
	session.Stdin = stdin
	session.Stdout = stdout
	session.Stderr = stderr

	err = session.Run(cmd)
	if err == nil {
		return nil
	}
	if exitErr, ok := err.(*ssh.ExitError); ok {
		return &ExitError{Code: exitErr.ExitStatus()}
	}
	return err
}

type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("remote command exited with status %d", e.Code)
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func (e *SSHExecutor) Upload(local, remote string) error {
	client, err := e.connect()
	if err != nil {
		return err
	}
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("sftp: %w", err)
	}
	defer sftpClient.Close()

	if err := ensureRemoteDir(sftpClient, filepath.Dir(remote)); err != nil {
		return err
	}
	src, err := os.Open(local)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := sftpClient.Create(remote)
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = io.Copy(dst, src)
	return err
}

func (e *SSHExecutor) UploadBytes(data []byte, remote string) error {
	client, err := e.connect()
	if err != nil {
		return err
	}
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return err
	}
	defer sftpClient.Close()
	if err := ensureRemoteDir(sftpClient, filepath.Dir(remote)); err != nil {
		return err
	}
	f, err := sftpClient.Create(remote)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

func (e *SSHExecutor) Download(remote string) ([]byte, error) {
	client, err := e.connect()
	if err != nil {
		return nil, err
	}
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return nil, err
	}
	defer sftpClient.Close()
	f, err := sftpClient.Open(remote)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func ensureRemoteDir(c *sftp.Client, dir string) error {
	dir = strings.ReplaceAll(dir, "\\", "/")
	parts := strings.Split(dir, "/")
	current := ""
	if strings.HasPrefix(dir, "/") {
		current = "/"
	}
	for _, part := range parts {
		if part == "" {
			continue
		}
		if current == "/" {
			current = "/" + part
		} else if current == "" {
			current = part
		} else {
			current = current + "/" + part
		}
		if _, err := c.Stat(current); err != nil {
			if err := c.Mkdir(current); err != nil {
				return err
			}
		}
	}
	return nil
}

type forwardCloser struct {
	listener net.Listener
	done     chan struct{}
}

func (f *forwardCloser) Close() error {
	close(f.done)
	return f.listener.Close()
}

func (e *SSHExecutor) Forward(ctx context.Context, spec ForwardSpec) (io.Closer, error) {
	client, err := e.connect()
	if err != nil {
		return nil, err
	}
	localHost := spec.LocalHost
	if localHost == "" {
		localHost = "127.0.0.1"
	}
	remoteHost := spec.RemoteHost
	if remoteHost == "" {
		remoteHost = "127.0.0.1"
	}
	addr := fmt.Sprintf("%s:%d", localHost, spec.LocalPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		if opErr, ok := err.(*net.OpError); ok && strings.Contains(opErr.Err.Error(), "address already in use") {
			return nil, fmt.Errorf("local port %d is already in use — try --local-port %d or stop the conflicting process", spec.LocalPort, spec.LocalPort+1000)
		}
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			default:
			}
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-done:
					return
				case <-ctx.Done():
					return
				default:
					continue
				}
			}
			go func(local net.Conn) {
				defer local.Close()
				remote, err := client.Dial("tcp", fmt.Sprintf("%s:%d", remoteHost, spec.RemotePort))
				if err != nil {
					return
				}
				defer remote.Close()
				go io.Copy(remote, local)
				io.Copy(local, remote)
			}(conn)
		}
	}()
	return &forwardCloser{listener: ln, done: done}, nil
}

func (e *SSHExecutor) Close() error {
	if e.client != nil {
		return e.client.Close()
	}
	return nil
}

func Ping(ctx context.Context, exec Executor) (time.Duration, error) {
	start := time.Now()
	var buf bytes.Buffer
	code, err := exec.Run(ctx, "echo outpost-ok", RunOpts{Stdout: &buf})
	if err != nil {
		return 0, err
	}
	if code != 0 || strings.TrimSpace(buf.String()) != "outpost-ok" {
		return 0, fmt.Errorf("unexpected ping response: %q", buf.String())
	}
	return time.Since(start), nil
}
