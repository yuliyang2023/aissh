// Package desktop exposes the local catalog, conversation history and terminal
// bridge through Wails. Credential secrets remain in the backend.
package desktop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/alerts"
	"git.example.com/ai/wrd-ai-cli/internal/catalog"
	"git.example.com/ai/wrd-ai-cli/internal/externals"
	"git.example.com/ai/wrd-ai-cli/internal/history"
	"git.example.com/ai/wrd-ai-cli/internal/resources"
	"git.example.com/ai/wrd-ai-cli/internal/sshauth"
	wails "github.com/wailsapp/wails/v2/pkg/runtime"
)

type Service struct {
	widgetMu sync.Mutex

	externalMu         sync.Mutex
	externalCalls      map[string]pendingExternalCall
	externalJobs       map[string]context.CancelFunc
	store              *catalog.Store
	history            *history.Store
	ctx                context.Context
	mu                 sync.Mutex
	catalogChatMu      sync.Mutex
	scans              map[string]pendingScan
	sessions           map[string]*terminalSession
	credentialRequests map[string]chan bool
	executor           *resources.Executor
	emit               func(string, ...any)
	alertMu            sync.Mutex
	alerts             *alerts.Engine
	alertError         string
	taskNotices        []PetTaskNotice // guarded by alertMu
	taskNoticeSequence uint64
	petRead            map[string]PetReadSequence // guarded by alertMu, scoped to each session
	monitorCancel      context.CancelFunc
	focusMain          func()
	petMu              sync.Mutex
	pet                *petProcess
}
type pendingScan struct {
	check   sshauth.HostCheck
	expires time.Time
}

func New(store *catalog.Store) *Service {
	dir := strings.TrimSpace(os.Getenv("AISSH_HISTORY_DIR"))
	if dir == "" {
		dir = history.DefaultDir()
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("AISSH_HISTORY"))) {
	case "0", "false", "off", "no":
		dir = ""
	}
	s := &Service{store: store, history: history.New(dir), ctx: context.Background(), scans: map[string]pendingScan{}, sessions: map[string]*terminalSession{}, credentialRequests: map[string]chan bool{}, executor: resources.New(store, "", "", nil), emit: func(string, ...any) {}}
	s.loadAlertPolicy()
	return s
}
func Startup(s *Service, ctx context.Context) {
	s.ctx = ctx
	s.emit = func(name string, args ...any) { wails.EventsEmit(ctx, name, args...) }
	s.focusMain = func() { wails.WindowShow(ctx); wails.WindowUnminimise(ctx) }
	monitorCtx, cancel := context.WithCancel(ctx)
	s.monitorCancel = cancel
	go s.runAlerts(monitorCtx)
}
func Shutdown(s *Service) {
	if s.monitorCancel != nil {
		s.monitorCancel()
	}
	s.stopPet()
	s.mu.Lock()
	sessions := make([]*terminalSession, 0, len(s.sessions))
	for _, session := range s.sessions {
		sessions = append(sessions, session)
	}
	s.mu.Unlock()
	for _, session := range sessions {
		session.close()
	}
}

func (s *Service) Snapshot() (catalog.Data, error) {
	d, err := s.store.Load()
	for i, r := range d.Resources {
		if r.Managed {
			if report, readErr := externals.NewStore(s.store).Discovery(r.ID); readErr == nil && report != nil {
				r.Discovery = report
				d.Resources[i].Discovery = report
			}
		}
		if r.Discovery != nil && catalog.CurrentResourceDiscovery(r, d) == nil {
			copy := *r.Discovery
			copy.Status, copy.Summary, copy.Operations = "stale", "连接、凭据或 SSH 路径已改变，请重新探索。", nil
			d.Resources[i].Discovery = &copy
		}
	}
	for i := range d.Credentials {
		d.Credentials[i].SecretRef = ""
	}
	return d, err
}
func (s *Service) SaveCredential(in catalog.CredentialInput) (catalog.Credential, error) {
	c, err := s.store.SaveCredential(in)
	c.SecretRef = ""
	return c, err
}
func (s *Service) SaveHost(h catalog.Host) (catalog.Host, error) { return s.store.SaveHost(h) }
func (s *Service) SaveResource(r catalog.Resource) (catalog.Resource, error) {
	saved, err := s.store.SaveResource(r)
	if err != nil {
		return saved, err
	}
	if _, err := externals.NewStore(s.store).Ensure(saved.ID); err != nil {
		return saved, err
	}
	d, err := s.store.Load()
	if err != nil {
		return saved, err
	}
	saved, _ = d.Resource(saved.ID)
	report, _ := externals.NewStore(s.store).Discovery(saved.ID)
	if report == nil || report.Binding != catalog.ResourceBinding(saved, d) {
		if _, err := s.StartResourceDiscovery(saved.ID); err != nil {
			return saved, err
		}
	}
	saved.Discovery = report
	return saved, nil
}
func (s *Service) SaveAIProfile(p catalog.AIProfile, makeDefault bool) (catalog.AIProfile, error) {
	return s.store.SaveAIProfile(p, makeDefault)
}
func (s *Service) Delete(kind, id string) error  { return s.store.Delete(kind, id) }
func (s *Service) CleanupSecrets() error         { return s.store.CleanupSecrets() }
func (s *Service) ClearRecentConnections() error { return s.store.ClearRecentConnections() }
func (s *Service) PickKeyFile() (string, error) {
	return wails.OpenFileDialog(s.ctx, wails.OpenDialogOptions{Title: "选择 SSH 私钥文件"})
}
func (s *Service) PickWorkspace() (string, error) {
	return wails.OpenDirectoryDialog(s.ctx, wails.OpenDialogOptions{Title: "选择本机 AI 工作区"})
}
func (s *Service) ImportSSH() ([]catalog.Host, error) {
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()
	return sshauth.ImportCandidates(ctx)
}

func (s *Service) CheckHost(id string) (sshauth.HostCheck, error) {
	helper, err := cliPath()
	if err != nil {
		return sshauth.HostCheck{}, err
	}
	check, err := sshauth.CheckChain(s.ctx, s.store, id, helper)
	if err != nil {
		return check, err
	}
	if !check.Known {
		check.Token = catalog.NewID()
		s.mu.Lock()
		for id, p := range s.scans {
			if time.Now().After(p.expires) {
				delete(s.scans, id)
			}
		}
		s.scans[check.Token] = pendingScan{check: check, expires: time.Now().Add(5 * time.Minute)}
		s.mu.Unlock()
	}
	// Raw keys stay in the trusted backend. TrustHost accepts only its opaque
	// challenge token, not a frontend-supplied known_hosts line.
	check.Keys = append([]sshauth.HostKey(nil), check.Keys...)
	for i := range check.Keys {
		check.Keys[i].Line = ""
	}
	return check, nil
}
func (s *Service) TrustHost(token string) error {
	s.mu.Lock()
	pending, ok := s.scans[token]
	delete(s.scans, token)
	s.mu.Unlock()
	if !ok || time.Now().After(pending.expires) {
		return errors.New("主机指纹确认已过期，请重新连接")
	}
	return sshauth.Trust(s.store, pending.check)
}
func (s *Service) TestHost(id string) error {
	helper, err := cliPath()
	if err != nil {
		return err
	}
	return sshauth.TestConnection(s.ctx, s.store, id, helper)
}
func (s *Service) RunResource(op resources.Operation) (string, error) {
	return "", errors.New("请在操作目录预览接口请求，再使用本次确认令牌执行")
}
func (s *Service) TestResource(id string) (string, error) {
	report, err := s.ExploreResource(id)
	if err != nil {
		return "", err
	}
	return report.Summary, nil
}
func cliPath() (string, error) {
	name := "aissh"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	candidates := []string{filepath.Join(filepath.Dir(executable), name), filepath.Join(filepath.Dir(executable), "..", "Resources", name)}
	// An explicit path is useful to developers, never accepted from a model or
	// frontend operation. Packaged apps normally use their bundled binary.
	if path := os.Getenv("AISSH_CLI_PATH"); path != "" {
		candidates = append([]string{path}, candidates...)
	}
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return filepath.Abs(path)
		}
	}
	return "", errors.New("未找到配套的 aissh 程序，请通过 make desktop 构建完整客户端")
}
