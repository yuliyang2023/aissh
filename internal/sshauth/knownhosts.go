package sshauth

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/catalog"
	"github.com/gofrs/flock"
	"golang.org/x/crypto/ssh"
)

type HostKey struct {
	Algorithm   string `json:"algorithm"`
	Fingerprint string `json:"fingerprint"`
	Line        string `json:"line"`
}
type HostCheck struct {
	HostID  string    `json:"hostId"`
	Name    string    `json:"name"`
	Address string    `json:"address"`
	Known   bool      `json:"known"`
	Keys    []HostKey `json:"keys"`
	Token   string    `json:"token"`
}

func HasKnownHost(ctx context.Context, store *catalog.Store, h catalog.Host) bool {
	home, _ := os.UserHomeDir()
	for _, path := range []string{KnownHostsPath(store), filepath.Join(home, ".ssh", "known_hosts"), "/etc/ssh/ssh_known_hosts"} {
		cmd := exec.CommandContext(ctx, "ssh-keygen", "-F", HostAddress(h), "-f", path)
		if out, err := cmd.Output(); err == nil && len(out) > 0 {
			return true
		}
	}
	return false
}

// CheckChain returns the first untrusted hop, from the outermost jump host to
// the destination. A GUI must show its fingerprints before Trust is called.
func CheckChain(ctx context.Context, store *catalog.Store, id, helper string) (HostCheck, error) {
	d, err := store.Load()
	if err != nil {
		return HostCheck{}, err
	}
	h, err := d.Host(id)
	if err != nil {
		return HostCheck{}, err
	}
	chain := []catalog.Host{h}
	for h.JumpHostID != "" {
		h, err = d.Host(h.JumpHostID)
		if err != nil {
			return HostCheck{}, err
		}
		chain = append(chain, h)
	}
	for i := len(chain) - 1; i >= 0; i-- {
		h = chain[i]
		if HasKnownHost(ctx, store, h) {
			continue
		}
		return scan(ctx, store, h, helper)
	}
	return HostCheck{HostID: id, Known: true, Keys: []HostKey{}}, nil
}
func scan(ctx context.Context, store *catalog.Store, h catalog.Host, helper string) (HostCheck, error) {
	result := HostCheck{HostID: h.ID, Name: h.Name, Address: HostAddress(h), Keys: []HostKey{}}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	lease, err := Prepare(ctx, store, h.ID, helper, nil)
	if err != nil {
		return result, err
	}
	defer lease.Close()
	dir, err := os.MkdirTemp("", "aissh-host-key-")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "known_hosts")
	// Use the same OpenSSH transport (including ProxyCommand/ProxyJump) as the
	// session. The candidate key goes ONLY into this disposable file, never a
	// trusted store. Disable destination authentication and all session actions;
	// only already-trusted jump hosts may use the lease's saved credentials.
	args := []string{"-N", "-T"}
	for _, option := range []string{
		"ConnectTimeout=8", "ConnectionAttempts=1", "BatchMode=yes",
		"PreferredAuthentications=none", "PubkeyAuthentication=no",
		"PasswordAuthentication=no", "KbdInteractiveAuthentication=no",
		"StrictHostKeyChecking=accept-new", "UserKnownHostsFile=" + configQuote(path),
		"GlobalKnownHostsFile=none", "KnownHostsCommand=none", "VerifyHostKeyDNS=no",
		"HashKnownHosts=no", "CheckHostIP=no", "UpdateHostKeys=no",
		"ControlPath=none", "ControlMaster=no", "ControlPersist=no",
		"ClearAllForwardings=yes", "ForwardAgent=no", "PermitLocalCommand=no", "RemoteCommand=none",
	} {
		args = append(args, "-o", option)
	}
	cmd := exec.CommandContext(ctx, "ssh", append(args, lease.Args...)...)
	cmd.Env = lease.Env
	// Let OpenSSH close its proxy children when the candidate has been read.
	// Windows does not support os.Interrupt; fall back to terminating there.
	cmd.Cancel = func() error {
		err := cmd.Process.Signal(os.Interrupt)
		if err != nil && !errors.Is(err, os.ErrProcessDone) {
			return cmd.Process.Kill()
		}
		return err
	}
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		return result, errors.New("无法启动 SSH 主机指纹检查")
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	defer func() { cancel(); <-done }()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		// Usually ssh exits after its disabled authentication fails. Poll too,
		// so a server allowing authentication method 'none' cannot keep -N open.
		select {
		case <-done:
			result.Keys = scannedKeys(path, h)
			if len(result.Keys) == 0 {
				return result, errors.New("无法读取主机指纹；请检查地址、网络、SSH 代理配置和跳板机")
			}
			return result, nil
		case <-ticker.C:
			if result.Keys = scannedKeys(path, h); len(result.Keys) > 0 {
				return result, nil
			}
		case <-ctx.Done():
			return result, errors.New("读取 SSH 主机指纹超时或已取消")
		}
	}
}

func scannedKeys(path string, h catalog.Host) []HostKey {
	out, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var keys []HostKey
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		_, _, key, _, _, err := ssh.ParseKnownHosts([]byte(line))
		if err != nil {
			continue
		}
		// Bind the candidate to the catalog's exact authority, independently
		// of user settings such as HostKeyAlias.
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		keys = append(keys, HostKey{Algorithm: key.Type(), Fingerprint: ssh.FingerprintSHA256(key), Line: HostAddress(h) + " " + fields[1] + " " + fields[2]})
	}
	return keys
}
func Trust(store *catalog.Store, check HostCheck) error {
	d, err := store.Load()
	if err != nil {
		return err
	}
	h, err := d.Host(check.HostID)
	if err != nil {
		return err
	}
	if HostAddress(h) != check.Address || len(check.Keys) == 0 {
		return errors.New("主机配置已变化，请重新检查指纹")
	}
	if err := os.MkdirAll(store.Dir, 0700); err != nil {
		return err
	}
	lock := flock.New(filepath.Join(store.Dir, "known_hosts.lock"))
	if err := lock.Lock(); err != nil {
		return err
	}
	defer lock.Unlock()
	f, err := os.OpenFile(KnownHostsPath(store), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, key := range check.Keys {
		_, hosts, pub, _, _, err := ssh.ParseKnownHosts([]byte(key.Line))
		if err != nil || len(hosts) != 1 || hosts[0] != HostAddress(h) || ssh.FingerprintSHA256(pub) != key.Fingerprint {
			return errors.New("无效的主机指纹")
		}
		if _, err := fmt.Fprintln(f, key.Line); err != nil {
			return err
		}
	}
	return f.Sync()
}

// ImportCandidates reads literal Host aliases, and lets OpenSSH itself resolve
// their settings. It never imports private key contents or modifies ssh/config.
func ImportCandidates(ctx context.Context) ([]catalog.Host, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	aliases := map[string]bool{}
	seen := map[string]bool{}
	var read func(string, int)
	read = func(path string, depth int) {
		if depth > 5 || seen[path] {
			return
		}
		seen[path] = true
		f, err := os.Open(path)
		if err != nil {
			return
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) < 2 {
				continue
			}
			switch strings.ToLower(fields[0]) {
			case "host":
				for _, alias := range fields[1:] {
					if !strings.ContainsAny(alias, "*?!#\"'") {
						aliases[alias] = true
					}
				}
			case "include":
				for _, pattern := range fields[1:] {
					pattern = expandPath(strings.Trim(pattern, "\""))
					if !filepath.IsAbs(pattern) {
						pattern = filepath.Join(home, ".ssh", pattern)
					}
					matches, _ := filepath.Glob(pattern)
					for _, match := range matches {
						read(match, depth+1)
					}
				}
			}
		}
	}
	read(filepath.Join(home, ".ssh", "config"), 0)
	result := []catalog.Host{}
	for alias := range aliases {
		if strings.HasPrefix(alias, "-") || len(result) >= 200 {
			continue
		}
		queryCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		out, err := exec.CommandContext(queryCtx, "ssh", "-G", alias).Output()
		cancel()
		if err != nil {
			continue
		}
		h := catalog.Host{Name: alias, Port: 22, Group: "SSH 导入"}
		for _, line := range strings.Split(string(out), "\n") {
			key, value, ok := strings.Cut(line, " ")
			if !ok {
				continue
			}
			switch key {
			case "hostname":
				h.Address = value
			case "user":
				h.User = value
			case "port":
				h.Port, _ = strconv.Atoi(value)
			}
		}
		result = append(result, h)
	}
	return result, nil
}
