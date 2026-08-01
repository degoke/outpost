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
	"sync"
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
	if cfg.PromptAuth || IsInteractive() {
		cfg.PromptAuth = true
	}
	return &SSHExecutor{cfg: cfg}, nil
}

func (e *SSHExecutor) connect() (*ssh.Client, error) {
	if e.client != nil {
		return e.client, nil
	}
	auth, err := buildAuth(e.cfg)
	if err != nil {
		return nil, err
	}
	sshCfg := &ssh.ClientConfig{
		User:            e.cfg.User,
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second,
	}
	if kh, err := hostKeyCallback(e.cfg); err == nil {
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
		if cfg.AuthMode == AuthPassword {
			return fmt.Errorf("authentication failed for %s@%s — check the password, or choose ssh private key if the server only allows public key login", cfg.User, cfg.Hostname)
		}
		if cfg.IdentityFile != "" {
			return fmt.Errorf("authentication failed for %s@%s using %s — verify the key is authorized in authorized_keys on the host", cfg.User, cfg.Hostname, cfg.IdentityFile)
		}
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
		stdinPipe, err := session.StdinPipe()
		if err != nil {
			return 1, err
		}
		if err := session.Start(cmd); err != nil {
			return 1, err
		}
		copyErr := make(chan error, 1)
		go func() {
			_, err := io.Copy(stdinPipe, opts.Stdin)
			closeErr := stdinPipe.Close()
			if err != nil {
				copyErr <- err
				return
			}
			copyErr <- closeErr
		}()
		errCh := make(chan error, 1)
		go func() { errCh <- session.Wait() }()
		select {
		case err = <-errCh:
		case <-ctx.Done():
			_ = session.Close()
			<-errCh
			return 1, ctx.Err()
		}
		if copyErr := <-copyErr; copyErr != nil {
			return 1, copyErr
		}
		if err == nil {
			return 0, nil
		}
		if exitErr, ok := err.(*ssh.ExitError); ok {
			return exitErr.ExitStatus(), nil
		}
		return 1, err
	}

	if err := session.Start(cmd); err != nil {
		return 1, err
	}
	errCh := make(chan error, 1)
	go func() { errCh <- session.Wait() }()
	select {
	case err = <-errCh:
	case <-ctx.Done():
		_ = session.Close()
		<-errCh
		return 1, ctx.Err()
	}
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

	return runInteractiveSession(ctx, session, cmd, stdin, stdout, stderr)
}

type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("remote command exited with status %d", e.Code)
}

// ExitStatus returns the remote exit code when err is a transport.ExitError.
func ExitStatus(err error) (int, bool) {
	if exitErr, ok := err.(*ExitError); ok {
		return exitErr.Code, true
	}
	return 0, false
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func (e *SSHExecutor) Upload(local, remote string) error {
	return e.UploadWithProgress(local, remote, nil)
}

func (e *SSHExecutor) UploadWithProgress(local, remote string, out io.Writer) error {
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
	info, err := src.Stat()
	if err != nil {
		return err
	}
	dst, err := sftpClient.Create(remote)
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = CopyWithProgress(dst, src, info.Size(), "Uploading to host", out, nil)
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

func (e *SSHExecutor) DownloadTo(local, remote string) error {
	return e.DownloadToWithProgress(local, remote, nil)
}

func (e *SSHExecutor) DownloadToWithProgress(local, remote string, out io.Writer) error {
	client, err := e.connect()
	if err != nil {
		return err
	}
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return err
	}
	defer sftpClient.Close()
	src, err := sftpClient.Open(remote)
	if err != nil {
		return err
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(local), 0755); err != nil {
		return err
	}
	dst, err := os.Create(local)
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = CopyWithProgress(dst, src, info.Size(), "Downloading from host", out, nil)
	return err
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
	once     sync.Once
}

func (f *forwardCloser) Close() error {
	var err error
	f.once.Do(func() {
		close(f.done)
		err = f.listener.Close()
	})
	return err
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
