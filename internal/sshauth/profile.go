package sshauth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/catalog"
	"git.example.com/ai/wrd-ai-cli/internal/redact"
)

type Lease struct {
	Args   []string
	Env    []string
	Host   catalog.Host
	Filter *redact.Filter
	close  func()
}

func (l *Lease) Close() {
	if l != nil && l.close != nil {
		l.close()
	}
}
func expandPath(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	return path
}
func configQuote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
func KnownHostsPath(s *catalog.Store) string { return filepath.Join(s.Dir, "known_hosts") }

func Prepare(ctx context.Context, store *catalog.Store, id, helper string, filter *redact.Filter) (*Lease, error) {
	d, err := store.Load()
	if err != nil {
		return nil, err
	}
	return PrepareSnapshot(ctx, store, d, id, helper, filter)
}

func PrepareSnapshot(ctx context.Context, store *catalog.Store, d catalog.Data, id, helper string, filter *redact.Filter) (*Lease, error) {
	host, err := d.Host(id)
	if err != nil {
		return nil, err
	}
	if filter == nil {
		filter = redact.New()
	}
	if helper == "" {
		helper, err = os.Executable()
		if err != nil {
			return nil, err
		}
	}
	helper, err = filepath.Abs(helper)
	if err != nil {
		return nil, err
	}
	if strings.ContainsAny(helper, "\x00\r\n") {
		return nil, errors.New("无效的认证助手路径")
	}
	dir, err := os.MkdirTemp("", "aissh-auth-")
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.RemoveAll(dir)
		}
	}()
	b := &broker{store: store, filter: filter, token: catalog.NewID()}
	var config strings.Builder
	chain := []catalog.Host{host}
	for current := host; current.JumpHostID != ""; {
		current, err = d.Host(current.JumpHostID)
		if err != nil {
			return nil, err
		}
		chain = append(chain, current)
	}
	home, _ := os.UserHomeDir()
	for _, h := range chain {
		user := h.User
		var credential catalog.Credential
		if h.CredentialID != "" {
			credential, err = d.Credential(h.CredentialID)
			if err != nil {
				return nil, err
			}
			if user == "" {
				user = credential.Username
			}
		}
		fmt.Fprintf(&config, "Host aissh-%s\n  HostName %s\n  Port %d\n", h.ID, configQuote(h.Address), h.Port)
		if user != "" {
			fmt.Fprintf(&config, "  User %s\n", configQuote(user))
		}
		fmt.Fprintf(&config, "  StrictHostKeyChecking yes\n  UserKnownHostsFile %s %s\n  ForwardAgent no\n  NumberOfPasswordPrompts 1\n", configQuote(KnownHostsPath(store)), configQuote(filepath.Join(home, ".ssh", "known_hosts")))
		if h.JumpHostID != "" {
			fmt.Fprintf(&config, "  ProxyJump aissh-%s\n", h.JumpHostID)
		}
		switch credential.Kind {
		case "password":
			if user == "" {
				return nil, errors.New("保存密码的 SSH 主机必须填写用户名")
			}
			fmt.Fprintln(&config, "  PreferredAuthentications password\n  PubkeyAuthentication no\n  KbdInteractiveAuthentication no")
			b.answers = append(b.answers, answer{credential: credential, prompts: []string{user + "@" + h.Address + "'s password:", user + "@aissh-" + h.ID + "'s password:"}})
		case "key":
			path, err := filepath.Abs(expandPath(credential.IdentityFile))
			if err != nil {
				return nil, err
			}
			info, err := os.Stat(path)
			if err != nil || info.IsDir() {
				return nil, errors.New("私钥文件不存在或不可读")
			}
			fmt.Fprintf(&config, "  IdentityFile %s\n  IdentitiesOnly yes\n", configQuote(path))
			if credential.HasSecret {
				b.answers = append(b.answers, answer{credential: credential, prompts: []string{"Enter passphrase for key '" + path + "':", "Enter passphrase for key '" + truncateKeyPrompt(path) + "':"}})
			}
		}
	}
	// Preserve global user settings (algorithms, agent socket, etc.). Managed
	// host/destination/identity settings above win under OpenSSH's first-value rule.
	fmt.Fprintf(&config, "Host *\n  Include %s\n", configQuote(filepath.Join(home, ".ssh", "config")))
	path := filepath.Join(dir, "ssh_config")
	if err := os.WriteFile(path, []byte(config.String()), 0600); err != nil {
		return nil, err
	}
	l := &Lease{Host: host, Filter: filter, Args: []string{"-F", path, "aissh-" + id}, Env: CleanEnvironment(os.Environ())}
	var server *http.Server
	if len(b.answers) > 0 {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		server = &http.Server{Handler: b, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 190 * time.Second, MaxHeaderBytes: 4096}
		go func() { _ = server.Serve(listener) }()
		// SSH_ASKPASS must name an executable, not a command string. The CLI
		// recognises AISSH_ASKPASS_MODE before parsing its normal arguments.
		l.Env = append(l.Env, "SSH_ASKPASS="+helper, "SSH_ASKPASS_REQUIRE=force", "AISSH_ASKPASS_MODE=1", askpassAddress+"="+listener.Addr().String(), askpassToken+"="+b.token)
	}
	l.close = func() {
		if server != nil {
			_ = server.Close()
		}
		_ = os.RemoveAll(dir)
	}
	ok = true
	return l, nil
}

func TestConnection(ctx context.Context, store *catalog.Store, id, helper string) error {
	lease, err := Prepare(ctx, store, id, helper, nil)
	if err != nil {
		return err
	}
	defer lease.Close()
	args := append([]string{"-T", "-o", "ConnectTimeout=8"}, lease.Args...)
	args = append(args, "true")
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ssh", args...)
	cmd.Env = lease.Env
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return errors.New("SSH 连接超时")
		}
		return fmt.Errorf("SSH 连接失败：%s", lease.Filter.Text(string(out)))
	}
	return nil
}

// Dial opens a direct-tcpip SSH channel without a listening TCP port or a
// plaintext credential on the SSH host. The adapter still verifies service TLS.
func Dial(ctx context.Context, store *catalog.Store, hostID, helper, target string, filter *redact.Filter) (net.Conn, error) {
	d, err := store.Load()
	if err != nil {
		return nil, err
	}
	return DialSnapshot(ctx, store, d, hostID, helper, target, filter)
}

func DialSnapshot(ctx context.Context, store *catalog.Store, d catalog.Data, hostID, helper, target string, filter *redact.Filter) (net.Conn, error) {
	if _, _, err := net.SplitHostPort(target); err != nil {
		return nil, errors.New("无效的隧道目标")
	}
	lease, err := PrepareSnapshot(ctx, store, d, hostID, helper, filter)
	if err != nil {
		return nil, err
	}
	args := append([]string{"-T", "-o", "ConnectTimeout=8", "-W", target}, lease.Args...)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	cmd.Env = lease.Env
	input, err := cmd.StdinPipe()
	if err != nil {
		lease.Close()
		return nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		lease.Close()
		return nil, err
	}
	// SSH diagnostics may contain server text, so keep them out of caller errors.
	if err := cmd.Start(); err != nil {
		lease.Close()
		return nil, errors.New("无法启动 SSH 隧道")
	}
	return &pipeConn{input: input, output: output, command: cmd, lease: lease}, nil
}

type pipeConn struct {
	input interface {
		Write([]byte) (int, error)
		Close() error
	}
	output interface {
		Read([]byte) (int, error)
		Close() error
	}
	command *exec.Cmd
	lease   *Lease
	once    sync.Once
}

func (c *pipeConn) Read(p []byte) (int, error)  { return c.output.Read(p) }
func (c *pipeConn) Write(p []byte) (int, error) { return c.input.Write(p) }
func (c *pipeConn) Close() error {
	c.once.Do(func() {
		_ = c.input.Close()
		_ = c.output.Close()
		if c.command.Process != nil {
			_ = c.command.Process.Kill()
		}
		_ = c.command.Wait()
		c.lease.Close()
	})
	return nil
}
func (c *pipeConn) LocalAddr() net.Addr              { return pipeAddr("local") }
func (c *pipeConn) RemoteAddr() net.Addr             { return pipeAddr("ssh") }
func (c *pipeConn) SetDeadline(time.Time) error      { return nil }
func (c *pipeConn) SetReadDeadline(time.Time) error  { return nil }
func (c *pipeConn) SetWriteDeadline(time.Time) error { return nil }

type pipeAddr string

func (p pipeAddr) Network() string { return "ssh" }
func (p pipeAddr) String() string  { return string(p) }

func HostAddress(h catalog.Host) string {
	if h.Port == 22 {
		return h.Address
	}
	return "[" + h.Address + "]:" + strconv.Itoa(h.Port)
}

// OpenSSH formats this particular prompt with %.100s. Match the exact formatted
// value, never an arbitrary prefix; the broker rejects ambiguous identities.
func truncateKeyPrompt(path string) string {
	if len(path) > 100 {
		// Helper JSON replaces each invalid UTF-8 byte at a truncated rune.
		return string([]rune(path[:100]))
	}
	return path
}
