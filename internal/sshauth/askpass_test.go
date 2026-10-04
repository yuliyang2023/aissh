package sshauth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/catalog"
	"git.example.com/ai/wrd-ai-cli/internal/redact"
	"golang.org/x/crypto/ssh"
)

func TestMain(m *testing.M) {
	if os.Getenv("AISSH_ASKPASS_MODE") == "1" {
		if len(os.Args) != 2 || RunHelper(os.Args[1], os.Stdout) != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type testSecrets map[string]string

func (m testSecrets) Get(k string) (string, error) {
	v, ok := m[k]
	if !ok {
		return "", errors.New("missing")
	}
	return v, nil
}
func (m testSecrets) Set(k, v string) error { m[k] = v; return nil }
func (m testSecrets) Delete(k string) error { delete(m, k); return nil }
func TestBrokerBindsPromptAndRejectsConfirmAndRetry(t *testing.T) {
	store := catalog.New(t.TempDir(), testSecrets{"ref": "sensitive-value"})
	b := &broker{store: store, filter: redact.New(), token: "capability", answers: []answer{{prompts: []string{"ops@host's password:"}, credential: catalog.Credential{SecretRef: "ref", HasSecret: true}}}}
	request := func(prompt, hint, token string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"prompt": prompt, "hint": hint})
		r := httptest.NewRequest("POST", "/answer", strings.NewReader(string(body)))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		b.ServeHTTP(w, r)
		return w
	}
	for _, attempt := range [][3]string{{"ops@host's password:", "confirm", "capability"}, {"root@other's password:", "", "capability"}, {"ops@host's password:", "", "wrong"}, {"Password:", "", "capability"}} {
		w := request(attempt[0], attempt[1], attempt[2])
		if w.Code != http.StatusForbidden || strings.Contains(w.Body.String(), "sensitive-value") {
			t.Fatal("unbound prompt received secret")
		}
	}
	w := request("ops@host's password:", "", "capability")
	if w.Code != 200 || w.Body.String() != "sensitive-value" {
		t.Fatal("bound prompt failed")
	}
	if request("ops@host's password:", "", "capability").Code != http.StatusForbidden {
		t.Fatal("secret retried")
	}
}
func TestCleanEnvironment(t *testing.T) {
	out := strings.Join(CleanEnvironment([]string{"PATH=/bin", "SSH_AUTH_SOCK=/tmp/agent", "DEEPSEEK_API_KEY=secret", "AISSH_AGENT_TOKEN=secret", "AISSH_ASKPASS_MODE=1", "SSH_ASKPASS=/evil"}), "\n")
	if strings.Contains(out, "secret") || strings.Contains(out, "ASKPASS") {
		t.Fatal("credentials inherited")
	}
	if !strings.Contains(out, "SSH_AUTH_SOCK") {
		t.Fatal("SSH agent lost")
	}
}

func startTestSSH(t *testing.T, callback func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error), public func(ssh.ConnMetadata, ssh.PublicKey) (*ssh.Permissions, error)) (net.Listener, ssh.PublicKey) {
	t.Helper()
	return startTestSSHServer(t, &ssh.ServerConfig{PasswordCallback: callback, PublicKeyCallback: public})
}

func startTestSSHServer(t *testing.T, config *ssh.ServerConfig) (net.Listener, ssh.PublicKey) {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, _ := ssh.NewSignerFromKey(private)
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			raw, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer raw.Close()
				conn, channels, requests, err := ssh.NewServerConn(raw, config)
				if err != nil {
					return
				}
				defer conn.Close()
				go ssh.DiscardRequests(requests)
				for channel := range channels {
					if channel.ChannelType() == "direct-tcpip" {
						go forwardTestChannel(channel)
						continue
					}
					if channel.ChannelType() != "session" {
						channel.Reject(ssh.UnknownChannelType, "unsupported")
						continue
					}
					stream, reqs, err := channel.Accept()
					if err != nil {
						continue
					}
					go func() {
						defer stream.Close()
						for req := range reqs {
							if req.Type == "exec" {
								req.Reply(true, nil)
								stream.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
								return
							}
							req.Reply(false, nil)
						}
					}()
				}
			}()
		}
	}()
	return listener, signer.PublicKey()
}

func forwardTestChannel(channel ssh.NewChannel) {
	var target struct {
		Host       string
		Port       uint32
		Origin     string
		OriginPort uint32
	}
	if ssh.Unmarshal(channel.ExtraData(), &target) != nil || target.Host != "127.0.0.1" {
		_ = channel.Reject(ssh.Prohibited, "test only permits loopback")
		return
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(target.Host, strconv.Itoa(int(target.Port))), time.Second)
	if err != nil {
		_ = channel.Reject(ssh.ConnectionFailed, "target unavailable")
		return
	}
	defer conn.Close()
	stream, requests, err := channel.Accept()
	if err != nil {
		return
	}
	defer stream.Close()
	go ssh.DiscardRequests(requests)
	go func() { _, _ = io.Copy(conn, stream); _ = conn.Close() }()
	_, _ = io.Copy(stream, conn)
}

func TestJumpHostAndPrivateServiceTunnel(t *testing.T) {
	if _, err := exec.LookPath("ssh"); err != nil {
		t.Skip("OpenSSH required")
	}
	store := catalog.New(t.TempDir(), testSecrets{})
	var hops []catalog.Host
	for _, user := range []string{"jump-user", "target-user"} {
		listener, key := startTestSSH(t, func(meta ssh.ConnMetadata, value []byte) (*ssh.Permissions, error) {
			if meta.User() == user && string(value) == user+"-password" {
				return nil, nil
			}
			return nil, errors.New("wrong credentials")
		}, nil)
		c, err := store.SaveCredential(catalog.CredentialInput{Credential: catalog.Credential{Name: user, Kind: "password", Username: user}, Secret: user + "-password"})
		if err != nil {
			t.Fatal(err)
		}
		h := catalog.Host{Name: user, Address: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port, CredentialID: c.ID}
		if len(hops) > 0 {
			h.JumpHostID = hops[0].ID
		}
		h, err = store.SaveHost(h)
		if err != nil {
			t.Fatal(err)
		}
		line := HostAddress(h) + " " + strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
		if err := Trust(store, HostCheck{HostID: h.ID, Address: HostAddress(h), Keys: []HostKey{{Line: line, Fingerprint: ssh.FingerprintSHA256(key)}}}); err != nil {
			t.Fatal(err)
		}
		hops = append(hops, h)
	}
	helper, _ := os.Executable()
	if err := TestConnection(context.Background(), store, hops[1].ID, helper); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "private-service-ok") }))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	transport := &http.Transport{DialContext: func(_ context.Context, _, address string) (net.Conn, error) {
		return Dial(ctx, store, hops[1].ID, helper, address, nil)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "private-service-ok" {
		t.Fatalf("tunnel body=%q err=%v", body, err)
	}
}
func TestOpenSSHPasswordAndHostVerification(t *testing.T) {
	if _, err := exec.LookPath("ssh"); err != nil {
		t.Skip("OpenSSH required")
	}
	listener, key := startTestSSH(t, func(meta ssh.ConnMetadata, secret []byte) (*ssh.Permissions, error) {
		if meta.User() == "ops" && string(secret) == "correct-test-password" {
			return nil, nil
		}
		return nil, errors.New("wrong credentials")
	}, nil)
	store := catalog.New(t.TempDir(), testSecrets{})
	c, err := store.SaveCredential(catalog.CredentialInput{Credential: catalog.Credential{Name: "SSH", Kind: "password", Username: "ops"}, Secret: "correct-test-password"})
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	host, err := store.SaveHost(catalog.Host{Name: "test", Address: "127.0.0.1", Port: port, CredentialID: c.ID})
	if err != nil {
		t.Fatal(err)
	}
	helper, _ := os.Executable()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := TestConnection(ctx, store, host.ID, helper); err == nil {
		t.Fatal("untrusted host accepted saved password")
	}
	line := HostAddress(host) + " " + strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))) + "\n"
	if err := os.WriteFile(KnownHostsPath(store), []byte(line), 0600); err != nil {
		t.Fatal(err)
	}
	if err := TestConnection(ctx, store, host.ID, helper); err != nil {
		t.Fatal(err)
	}
}
func TestOpenSSHEncryptedPrivateKey(t *testing.T) {
	if _, err := exec.LookPath("ssh"); err != nil {
		t.Skip("OpenSSH required")
	}
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	pub, _ := ssh.NewPublicKey(public)
	listener, key := startTestSSH(t, nil, func(meta ssh.ConnMetadata, offered ssh.PublicKey) (*ssh.Permissions, error) {
		if string(offered.Marshal()) == string(pub.Marshal()) {
			return nil, nil
		}
		return nil, errors.New("wrong key")
	})
	store := catalog.New(t.TempDir(), testSecrets{})
	block, err := ssh.MarshalPrivateKeyWithPassphrase(private, "test", []byte("test-passphrase"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.Dir, "test-key")
	os.WriteFile(path, pem.EncodeToMemory(block), 0600)
	c, err := store.SaveCredential(catalog.CredentialInput{Credential: catalog.Credential{Name: "private", Kind: "key", Username: "ops", IdentityFile: path}, Secret: "test-passphrase"})
	if err != nil {
		t.Fatal(err)
	}
	host, err := store.SaveHost(catalog.Host{Name: "test", Address: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port, CredentialID: c.ID})
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(KnownHostsPath(store), []byte(HostAddress(host)+" "+strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))+"\n"), 0600)
	helper, _ := os.Executable()
	if err := TestConnection(context.Background(), store, host.ID, helper); err != nil {
		t.Fatal(err)
	}
	// Reusing one encrypted private key for both the jump and target requires
	// two identical askpass prompts, each bound to the same credential.
	jumpListener, jumpKey := startTestSSH(t, nil, func(_ ssh.ConnMetadata, offered ssh.PublicKey) (*ssh.Permissions, error) {
		if string(offered.Marshal()) == string(pub.Marshal()) {
			return nil, nil
		}
		return nil, errors.New("wrong key")
	})
	jump, err := store.SaveHost(catalog.Host{Name: "jump", Address: "127.0.0.1", Port: jumpListener.Addr().(*net.TCPAddr).Port, CredentialID: c.ID})
	if err != nil {
		t.Fatal(err)
	}
	line := HostAddress(jump) + " " + strings.TrimSpace(string(ssh.MarshalAuthorizedKey(jumpKey)))
	if err := Trust(store, HostCheck{HostID: jump.ID, Address: HostAddress(jump), Keys: []HostKey{{Line: line, Fingerprint: ssh.FingerprintSHA256(jumpKey)}}}); err != nil {
		t.Fatal(err)
	}
	host.JumpHostID = jump.ID
	if _, err := store.SaveHost(host); err != nil {
		t.Fatal(err)
	}
	if err := TestConnection(context.Background(), store, host.ID, helper); err != nil {
		t.Fatal(err)
	}
}

func TestHelperRejectsNonLoopbackEndpoint(t *testing.T) {
	t.Setenv(askpassAddress, "evil.example:443")
	t.Setenv(askpassToken, strings.Repeat("a", 32))
	if err := RunHelper("password", io.Discard); err == nil {
		t.Fatal("accepted external credential broker")
	}
}
