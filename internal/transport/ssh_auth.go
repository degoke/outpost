package transport

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

var defaultIdentityCandidates = []string{
	"id_ed25519",
	"id_rsa",
	"id_ecdsa",
}

// DefaultIdentityFile returns the first existing private key in ~/.ssh.
func DefaultIdentityFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	for _, name := range defaultIdentityCandidates {
		path := filepath.Join(home, ".ssh", name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("no SSH private key found in ~/.ssh — specify --identity-file or use --auth password")
}

// IsInteractive reports whether stdin is a terminal.
func IsInteractive() bool {
	return interactiveCheck()
}

var interactiveCheck = func() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func buildAuth(cfg SSHConfig) ([]ssh.AuthMethod, error) {
	mode := cfg.AuthMode
	if mode == "" {
		mode = AuthAuto
	}

	var methods []ssh.AuthMethod
	useKeys, usePassword, err := authPlan(mode, cfg.IdentityFile)
	if err != nil {
		return nil, err
	}

	if useKeys {
		identityFile := cfg.IdentityFile
		if identityFile == "" {
			path, err := DefaultIdentityFile()
			if err != nil {
				return nil, err
			}
			identityFile = path
		}
		signer, err := loadSigner(identityFile, cfg.Passphrase, cfg.PromptAuth)
		if err != nil {
			return nil, err
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}

	if usePassword {
		methods = append(methods, passwordAuthMethods(cfg)...)
	}

	if len(methods) == 0 {
		return nil, fmt.Errorf("no SSH authentication method configured — use --auth password, --auth key with --identity-file, or pass --password")
	}
	return methods, nil
}

func authPlan(mode AuthMode, identityFile string) (useKeys, usePassword bool, err error) {
	switch mode {
	case AuthPassword:
		return false, true, nil
	case AuthKey:
		return true, false, nil
	case AuthAuto:
		if identityFile != "" {
			return true, true, nil
		}
		return false, true, nil
	default:
		return false, false, fmt.Errorf("unknown auth mode %q", mode)
	}
}

func passwordAuthMethods(cfg SSHConfig) []ssh.AuthMethod {
	label := fmt.Sprintf("SSH password for %s@%s", cfg.User, cfg.Hostname)
	var (
		password string
		passErr  error
		once     sync.Once
	)
	getPassword := func() (string, error) {
		once.Do(func() {
			if cfg.Password != "" {
				password = cfg.Password
				return
			}
			if cfg.PromptAuth {
				password, passErr = promptSecret(label)
				return
			}
			passErr = fmt.Errorf("%s required — re-run interactively or pass --password", label)
		})
		return password, passErr
	}

	return []ssh.AuthMethod{
		ssh.KeyboardInteractive(func(_ string, _ string, questions []string, echos []bool) ([]string, error) {
			if len(questions) == 0 {
				return []string{}, nil
			}
			pass, err := getPassword()
			if err != nil {
				return nil, err
			}
			answers := make([]string, len(questions))
			for i, echo := range echos {
				if echo {
					answers[i], err = promptVisibleAnswer(questions[i])
					if err != nil {
						return nil, err
					}
					continue
				}
				answers[i] = pass
			}
			return answers, nil
		}),
	}
}

func promptVisibleAnswer(question string) (string, error) {
	if !IsInteractive() {
		return "", fmt.Errorf("interactive answer required for %q", question)
	}
	fmt.Fprintf(os.Stderr, "%s ", strings.TrimSpace(question))
	return readInputLine()
}

func loadSigner(path string, passphrase []byte, prompt bool) (ssh.Signer, error) {
	path = expandPath(path)
	key, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("identity file not found at %s", path)
		}
		return nil, fmt.Errorf("read identity file %s: %w", path, err)
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err == nil {
		return signer, nil
	}
	if !isEncryptedKeyError(err) {
		return nil, fmt.Errorf("parse identity file %s: %w", path, err)
	}

	if len(passphrase) == 0 && prompt && IsInteractive() {
		passphrase, err = promptPassphrase(path)
		if err != nil {
			return nil, err
		}
	}
	if len(passphrase) == 0 {
		return nil, fmt.Errorf("identity file %s is passphrase-protected — enter the key passphrase or use --auth password for server password login", path)
	}

	signer, err = ssh.ParsePrivateKeyWithPassphrase(key, passphrase)
	if err != nil {
		return nil, fmt.Errorf("unlock identity file %s: %w", path, err)
	}
	return signer, nil
}

func isEncryptedKeyError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "passphrase") ||
		strings.Contains(msg, "encrypted") ||
		strings.Contains(msg, "cannot decode")
}

func promptPassphrase(path string) ([]byte, error) {
	fmt.Fprintf(os.Stderr, "Key passphrase for %s: ", path)
	pass, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("read key passphrase: %w", err)
	}
	if len(pass) == 0 {
		return nil, fmt.Errorf("key passphrase is required for %s", path)
	}
	return pass, nil
}

func promptSecret(label string) (string, error) {
	if !IsInteractive() {
		return "", fmt.Errorf("%s required — re-run interactively or pass --password", label)
	}
	fmt.Fprintf(os.Stderr, "%s: ", label)
	if term.IsTerminal(int(syscall.Stdin)) {
		pass, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		return string(pass), nil
	}
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// BuildAuthForTest exposes auth method construction for unit tests.
func BuildAuthForTest(cfg SSHConfig) ([]ssh.AuthMethod, error) {
	return buildAuth(cfg)
}

// AuthSelection is the resolved SSH authentication method for a connection.
type AuthSelection struct {
	Mode         AuthMode
	IdentityFile string
}

// ResolveAuthSelection picks how to authenticate. When auth is not explicitly set
// and no identity file is given, it prompts interactively for password vs key.
func ResolveAuthSelection(cfg SSHConfig, authExplicit bool) (AuthSelection, error) {
	mode := cfg.AuthMode
	if mode == "" {
		mode = AuthAuto
	}
	identity := strings.TrimSpace(cfg.IdentityFile)

	if identity != "" && !authExplicit {
		return finishAuthSelection(AuthKey, identity)
	}
	if authExplicit {
		return finishAuthSelection(mode, identity)
	}
	if !IsInteractive() {
		return AuthSelection{}, fmt.Errorf("SSH auth not specified — pass --auth password, --auth key with --identity-file, or run interactively")
	}
	return promptAuthSelection(cfg.User, cfg.Hostname)
}

func finishAuthSelection(mode AuthMode, identityFile string) (AuthSelection, error) {
	switch mode {
	case AuthPassword:
		return AuthSelection{Mode: AuthPassword}, nil
	case AuthKey:
		path, err := resolveIdentityFile(identityFile)
		if err != nil {
			return AuthSelection{}, err
		}
		return AuthSelection{Mode: AuthKey, IdentityFile: path}, nil
	case AuthAuto:
		if identityFile != "" {
			return AuthSelection{Mode: AuthKey, IdentityFile: expandPath(identityFile)}, nil
		}
		return AuthSelection{Mode: AuthPassword}, nil
	default:
		return AuthSelection{}, fmt.Errorf("unknown auth mode %q", mode)
	}
}

func promptAuthSelection(user, hostname string) (AuthSelection, error) {
	target := fmt.Sprintf("%s@%s", user, hostname)
	fmt.Fprintf(os.Stderr, "How should Outpost authenticate to %s?\n", target)
	fmt.Fprintln(os.Stderr, "  1) password")
	fmt.Fprintln(os.Stderr, "  2) ssh private key")
	fmt.Fprint(os.Stderr, "Choice (1/2) [1]: ")

	choice, err := readInputLine()
	if err != nil {
		return AuthSelection{}, err
	}
	if choice == "" || choice == "1" {
		return AuthSelection{Mode: AuthPassword}, nil
	}
	if choice == "2" {
		path, err := resolveIdentityFile("")
		if err != nil {
			return AuthSelection{}, err
		}
		return AuthSelection{Mode: AuthKey, IdentityFile: path}, nil
	}
	return AuthSelection{}, fmt.Errorf("invalid choice %q — enter 1 or 2", choice)
}

func resolveIdentityFile(path string) (string, error) {
	if path != "" {
		return expandPath(path), nil
	}
	defaultPath, defaultErr := DefaultIdentityFile()
	prompt := "Private key path"
	if defaultErr == nil {
		prompt = fmt.Sprintf("Private key path [%s]", defaultPath)
	}
	fmt.Fprint(os.Stderr, prompt+": ")
	line, err := readInputLine()
	if err != nil {
		return "", err
	}
	if line == "" {
		if defaultErr != nil {
			return "", defaultErr
		}
		return defaultPath, nil
	}
	path = expandPath(line)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("identity file not found at %s", path)
		}
		return "", fmt.Errorf("identity file %s: %w", path, err)
	}
	return path, nil
}

func readInputLine() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read input: %w", err)
	}
	return strings.TrimSpace(line), nil
}

// SetInteractiveForTest overrides interactive detection. Returns a restore function.
func SetInteractiveForTest(fn func() bool) func() {
	prev := interactiveCheck
	interactiveCheck = fn
	return func() { interactiveCheck = prev }
}

// ResolvedIdentityFile returns the key path used for key-based auth, if any.
func ResolvedIdentityFile(cfg SSHConfig) (string, error) {
	useKeys, _, err := authPlan(cfg.AuthMode, cfg.IdentityFile)
	if err != nil || !useKeys {
		return "", nil
	}
	if cfg.IdentityFile != "" {
		return expandPath(cfg.IdentityFile), nil
	}
	return DefaultIdentityFile()
}
