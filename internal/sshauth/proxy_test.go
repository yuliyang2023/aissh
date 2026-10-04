package sshauth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/catalog"
	"golang.org/x/crypto/ssh"
)

func sshConfigHome(t *testing.T, config string) string {
	t.Helper()
	if _, err := exec.LookPath("ssh"); err != nil {
		t.Skip("OpenSSH required")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.Mkdir(filepath.Join(home, ".ssh"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".ssh", "config")
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func resolvedSettings(t *testing.T, lease *Lease, target string) map[string]string {
	t.Helper()
	args := append([]string{"-G", "-T"}, lease.Args[:len(lease.Args)-1]...)
	cmd := exec.Command("ssh", append(args, target)...)
	cmd.Env = lease.Env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ssh -G: %v\n%s", err, out)
	}
	settings := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		key, value, ok := strings.Cut(line, " ")
		if ok {
			settings[key] = value
		}
	}
	return settings
}

func TestManagedConfigMatchesDestinationPatterns(t *testing.T) {
	sshConfigHome(t, `Host *.xxx.com !excluded.xxx.com
  ProxyCommand domain-proxy %h %p
  User config-user
  Port 1
  ServerAliveInterval 42
Host *
  ProxyCommand global-proxy %h %p
  ServerAliveInterval 99
`)
	store := catalog.New(t.TempDir(), testSecrets{})
	for _, tc := range []struct{ address, proxy, keepalive string }{
		{"server.xxx.com", "domain-proxy %h %p", "42"},
		{"excluded.xxx.com", "global-proxy %h %p", "99"},
		{"unrelated.example", "global-proxy %h %p", "99"},
	} {
		t.Run(tc.address, func(t *testing.T) {
			h, err := store.SaveHost(catalog.Host{Address: tc.address, User: "saved-user", Port: 2222})
			if err != nil {
				t.Fatal(err)
			}
			lease, err := Prepare(context.Background(), store, h.ID, "", nil)
			if err != nil {
				t.Fatal(err)
			}
			defer lease.Close()
			settings := resolvedSettings(t, lease, lease.Args[len(lease.Args)-1])
			for key, want := range map[string]string{"hostname": tc.address, "proxycommand": tc.proxy, "user": "saved-user", "port": "2222", "serveraliveinterval": tc.keepalive, "stricthostkeychecking": "true"} {
				if settings[key] != want {
					t.Errorf("%s = %q, want %q", key, settings[key], want)
				}
			}
		})
	}
}

func TestManagedJumpWinsAndMatchesProxyOnOuterHop(t *testing.T) {
	sshConfigHome(t, "Host *.xxx.com\n  ProxyCommand domain-proxy %h %p\n")
	store := catalog.New(t.TempDir(), testSecrets{})
	jump, err := store.SaveHost(catalog.Host{Address: "jump.xxx.com", User: "jump", Port: 2200})
	if err != nil {
		t.Fatal(err)
	}
	target, err := store.SaveHost(catalog.Host{Address: "target.xxx.com", User: "target", Port: 2222, JumpHostID: jump.ID})
	if err != nil {
		t.Fatal(err)
	}
	lease, err := Prepare(context.Background(), store, target.ID, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	settings := resolvedSettings(t, lease, "aissh-"+target.ID)
	if settings["proxyjump"] != "aissh-"+jump.ID || settings["proxycommand"] != "" {
		t.Fatalf("managed jump did not take priority: %v", settings)
	}
	settings = resolvedSettings(t, lease, "aissh-"+jump.ID)
	if settings["proxycommand"] != "domain-proxy %h %p" || settings["port"] != "2200" || settings["user"] != "jump" {
		t.Fatalf("jump settings lost: %v", settings)
	}
}

func TestUserProxyJumpAliasResolvesWithManagedConfig(t *testing.T) {
	sshConfigHome(t, `Host *.xxx.com
  ProxyJump relay
Host relay
  HostName relay.example.net
  Port 2201
  User relay-user
  ProxyCommand relay-proxy %h %p
`)
	store := catalog.New(t.TempDir(), testSecrets{})
	h, err := store.SaveHost(catalog.Host{Address: "target.xxx.com", Port: 2222})
	if err != nil {
		t.Fatal(err)
	}
	lease, err := Prepare(context.Background(), store, h.ID, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	settings := resolvedSettings(t, lease, "aissh-"+h.ID)
	if settings["proxyjump"] != "relay" {
		t.Fatalf("proxyjump = %q", settings["proxyjump"])
	}
	settings = resolvedSettings(t, lease, "relay")
	for key, want := range map[string]string{"hostname": "relay.example.net", "port": "2201", "user": "relay-user", "proxycommand": "relay-proxy %h %p"} {
		if settings[key] != want {
			t.Errorf("relay %s = %q, want %q", key, settings[key], want)
		}
	}
}

func TestFingerprintAndSessionUseProxyCommand(t *testing.T) {
	if _, err := exec.LookPath("nc"); err != nil {
		t.Skip("netcat required for test proxy")
	}
	for _, route := range []string{"direct", "proxy", "proxy-and-jump"} {
		t.Run(route, func(t *testing.T) {
			sshConfigHome(t, "Host *.xxx.com\n  ProxyCommand nc 127.0.0.1 %p\n")
			store := catalog.New(t.TempDir(), testSecrets{})
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			var authentications atomic.Int32
			listener, key := startTestSSH(t, func(meta ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
				authentications.Add(1)
				if meta.User() == "ops" && string(password) == "target-password" {
					return nil, nil
				}
				return nil, errors.New("wrong credentials")
			}, nil)
			credential, err := store.SaveCredential(catalog.CredentialInput{Credential: catalog.Credential{Name: "target", Kind: "password", Username: "ops"}, Secret: "target-password"})
			if err != nil {
				t.Fatal(err)
			}
			h := catalog.Host{Address: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port, CredentialID: credential.ID}
			if route == "proxy" {
				h.Address = "target.xxx.com"
			}
			var jump catalog.Host
			var jumpKey ssh.PublicKey
			if route == "proxy-and-jump" {
				var jumpListener net.Listener
				jumpListener, jumpKey = startTestSSH(t, func(_ ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
					if string(password) == "jump-password" {
						return nil, nil
					}
					return nil, errors.New("wrong jump password")
				}, nil)
				c, err := store.SaveCredential(catalog.CredentialInput{Credential: catalog.Credential{Name: "jump", Kind: "password", Username: "jump"}, Secret: "jump-password"})
				if err != nil {
					t.Fatal(err)
				}
				jump, err = store.SaveHost(catalog.Host{Address: "jump.xxx.com", Port: jumpListener.Addr().(*net.TCPAddr).Port, CredentialID: c.ID})
				if err != nil {
					t.Fatal(err)
				}
				h.JumpHostID = jump.ID
			}
			h, err = store.SaveHost(h)
			if err != nil {
				t.Fatal(err)
			}
			if route == "proxy-and-jump" {
				check, err := CheckChain(ctx, store, h.ID, "")
				if err != nil {
					t.Fatal(err)
				}
				if check.HostID != jump.ID || len(check.Keys) != 1 || check.Keys[0].Fingerprint != ssh.FingerprintSHA256(jumpKey) {
					t.Fatalf("wrong outer hop: %+v", check)
				}
				if err := Trust(store, check); err != nil {
					t.Fatal(err)
				}
			}
			check, err := CheckChain(ctx, store, h.ID, "")
			if err != nil {
				t.Fatal(err)
			}
			if check.Known || check.HostID != h.ID || len(check.Keys) != 1 || check.Keys[0].Fingerprint != ssh.FingerprintSHA256(key) {
				t.Fatalf("wrong candidate: %+v", check)
			}
			if authentications.Load() != 0 {
				t.Fatal("fingerprint scan sent destination credentials")
			}
			if HasKnownHost(ctx, store, h) {
				t.Fatal("fingerprint scan persisted trust without confirmation")
			}
			if err := TestConnection(ctx, store, h.ID, ""); err == nil {
				t.Fatal("session accepted an untrusted destination")
			}
			if authentications.Load() != 0 {
				t.Fatal("untrusted session sent destination credentials")
			}
			if err := Trust(store, check); err != nil {
				t.Fatal(err)
			}
			if err := TestConnection(ctx, store, h.ID, ""); err != nil {
				t.Fatal(err)
			}
			if authentications.Load() != 1 {
				t.Fatalf("destination authentication count = %d", authentications.Load())
			}
			if route == "proxy" {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "proxy-tunnel-ok") }))
				defer server.Close()
				transport := &http.Transport{DialContext: func(ctx context.Context, _, address string) (net.Conn, error) {
					return Dial(ctx, store, h.ID, "", address, nil)
				}}
				defer transport.CloseIdleConnections()
				client := &http.Client{Transport: transport}
				response, err := client.Get(server.URL)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				body, err := io.ReadAll(response.Body)
				if err != nil || string(body) != "proxy-tunnel-ok" {
					t.Fatalf("tunnel = %q, %v", body, err)
				}
			}
		})
	}
}

func TestFingerprintScanEndsWhenServerNeedsNoAuthentication(t *testing.T) {
	sshConfigHome(t, "")
	listener, key := startTestSSHServer(t, &ssh.ServerConfig{NoClientAuth: true})
	store := catalog.New(t.TempDir(), testSecrets{})
	h, err := store.SaveHost(catalog.Host{Address: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	check, err := CheckChain(ctx, store, h.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(check.Keys) != 1 || check.Keys[0].Fingerprint != ssh.FingerprintSHA256(key) {
		t.Fatalf("wrong candidate: %+v", check)
	}
}

func TestProxySessionRejectsChangedHostKey(t *testing.T) {
	if _, err := exec.LookPath("nc"); err != nil {
		t.Skip("netcat required for test proxy")
	}
	config := sshConfigHome(t, "")
	store := catalog.New(t.TempDir(), testSecrets{})
	h, err := store.SaveHost(catalog.Host{Address: "target.xxx.com", Port: 22})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for i := 0; i < 2; i++ {
		listener, _ := startTestSSHServer(t, &ssh.ServerConfig{NoClientAuth: true})
		content := fmt.Sprintf("Host *.xxx.com\n  ProxyCommand nc 127.0.0.1 %d\n", listener.Addr().(*net.TCPAddr).Port)
		if err := os.WriteFile(config, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			check, err := CheckChain(ctx, store, h.ID, "")
			if err != nil {
				t.Fatal(err)
			}
			if err := Trust(store, check); err != nil {
				t.Fatal(err)
			}
			if err := TestConnection(ctx, store, h.ID, ""); err != nil {
				t.Fatal(err)
			}
		} else if err := TestConnection(ctx, store, h.ID, ""); err == nil {
			t.Fatal("proxy destination host key change was accepted")
		}
	}
}
