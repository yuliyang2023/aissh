package desktop

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/alerts"
	wails "github.com/wailsapp/wails/v2/pkg/runtime"
)

const petAddressEnv = "AISSH_PET_ADDRESS"
const petTokenEnv = "AISSH_PET_TOKEN"

type petProcess struct {
	cancel context.CancelFunc
	server *http.Server
}

// DesktopReady runs after the main frontend has installed its event handlers.
func DesktopReady(s *Service) {
	if s.GetMonitoring().Policy.PetEnabled {
		if err := s.startPet(); err != nil {
			s.alertMu.Lock()
			s.alertError = "桌宠启动失败：" + err.Error()
			s.alertMu.Unlock()
		}
	}
}

func (s *Service) SetPetEnabled(enabled bool) error {
	if enabled {
		if err := s.startPet(); err != nil {
			return err
		}
	}
	s.alertMu.Lock()
	p := s.alerts.Policy
	p.PetEnabled = enabled
	err := saveAlertPolicy(s.store.Dir, p)
	if err == nil {
		s.alerts.Policy = p
		s.alertError = ""
	}
	s.alertMu.Unlock()
	if !enabled || err != nil {
		s.stopPet()
	}
	return err
}

func (s *Service) startPet() error {
	s.petMu.Lock()
	defer s.petMu.Unlock()
	if s.pet != nil {
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		listener.Close()
		return err
	}
	token := hex.EncodeToString(secret)
	server := &http.Server{Handler: s.petHandler(token), ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 10 * time.Second}
	ctx, cancel := context.WithCancel(s.ctx)
	cmd := exec.CommandContext(ctx, executable, "--desktop-pet")
	cmd.Env = append(desktopEnvironment(), petAddressEnv+"="+listener.Addr().String(), petTokenEnv+"="+token)
	if err := cmd.Start(); err != nil {
		cancel()
		listener.Close()
		return err
	}
	p := &petProcess{cancel: cancel, server: server}
	s.pet = p
	go func() { _ = server.Serve(listener) }()
	go func() {
		_ = cmd.Wait()
		_ = server.Close()
		cancel()
		s.petMu.Lock()
		unexpected := s.pet == p
		if unexpected {
			s.pet = nil
		}
		s.petMu.Unlock()
		if unexpected {
			s.alertMu.Lock()
			if s.alerts.Policy.PetEnabled {
				s.alertError = "桌宠已退出，可点击开启桌宠重新打开；后台监控仍在运行。"
			}
			s.alertMu.Unlock()
		}
	}()
	return nil
}

func (s *Service) stopPet() {
	s.petMu.Lock()
	p := s.pet
	s.pet = nil
	s.petMu.Unlock()
	if p != nil {
		_ = p.server.Close()
		p.cancel()
	}
}

type petAction struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// The helper receives only alert metadata. It cannot access the catalog, SSH
// terminal, workbench commands or credentials. Browsers cannot use this channel.
func (s *Service) petHandler(token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		if r.URL.Path == "/state" && r.Method == http.MethodGet {
			state := s.GetMonitoring()
			// Rules may contain user-provided log keywords; the pet only needs
			// enabled/recovery flags, counts and incident summaries.
			state.Policy.Rules = nil
			state.Monitors = nil
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(state)
			return
		}
		if r.URL.Path != "/action" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var a petAction
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&a) != nil {
			http.Error(w, "invalid action", 400)
			return
		}
		var err error
		switch a.Kind {
		case "open":
			err = s.OpenMonitorSession(a.ID)
		case "acknowledge":
			err = s.AcknowledgeAlert(a.ID)
		case "snooze":
			err = s.SnoozeAlerts(15)
		case "resume":
			err = s.SnoozeAlerts(0)
		case "hide":
			// Let the HTTP reply complete before terminating its client.
			s.alertMu.Lock()
			p := s.alerts.Policy
			p.PetEnabled = false
			err = saveAlertPolicy(s.store.Dir, p)
			if err == nil {
				s.alerts.Policy = p
			}
			s.alertMu.Unlock()
		default:
			err = errors.New("未知的桌宠操作")
		}
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// PetBridge is the only Wails binding exposed by the companion process.
type PetBridge struct {
	address, token string
	client         *http.Client
	ctx            context.Context
}

func NewPetBridge() (*PetBridge, error) {
	address, token := os.Getenv(petAddressEnv), os.Getenv(petTokenEnv)
	_ = os.Unsetenv(petAddressEnv)
	_ = os.Unsetenv(petTokenEnv)
	host, _, err := net.SplitHostPort(address)
	if err != nil || host != "127.0.0.1" || len(token) != 64 {
		return nil, errors.New("桌宠需要由主客户端启动")
	}
	return &PetBridge{address: address, token: token, client: &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func PetStartup(b *PetBridge, ctx context.Context) {
	b.ctx = ctx
	// This watchdog also covers a suspended or failed webview.
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		failures := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := b.Snapshot(); err != nil {
					failures++
				} else {
					failures = 0
				}
				if failures >= 3 {
					wails.Quit(ctx)
					return
				}
			}
		}
	}()
}

func (b *PetBridge) request(method, path, body string) (*http.Response, error) {
	req, err := http.NewRequest(method, "http://"+b.address+path, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+b.token)
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, errors.New("与主客户端的连接已中断")
	}
	if resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, errors.New("操作未完成，请回到主客户端查看当前状态")
	}
	return resp, nil
}
func (b *PetBridge) Snapshot() (alerts.State, error) {
	resp, err := b.request(http.MethodGet, "/state", "")
	if err != nil {
		return alerts.State{}, err
	}
	defer resp.Body.Close()
	var state alerts.State
	err = json.NewDecoder(resp.Body).Decode(&state)
	return state, err
}
func (b *PetBridge) Action(kind, id string) error {
	data, _ := json.Marshal(petAction{Kind: kind, ID: id})
	resp, err := b.request(http.MethodPost, "/action", string(data))
	if err != nil {
		return err
	}
	resp.Body.Close()
	if kind == "hide" {
		wails.Quit(b.ctx)
	}
	return nil
}
