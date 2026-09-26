package desktop

import (
	"context"
	"errors"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/catalog"
	"git.example.com/ai/wrd-ai-cli/internal/externals"
	"git.example.com/ai/wrd-ai-cli/internal/resources"
)

type pendingExternalCall struct {
	Executor *resources.Executor
	Expires  time.Time
}
type ExternalView struct {
	State       externals.State            `json:"state"`
	Job         externals.DiscoveryJob     `json:"job"`
	Discovery   *catalog.ResourceDiscovery `json:"discovery"`
	Invocations []externals.Invocation     `json:"invocations"`
}

func (s *Service) GetExternalState(id string) (ExternalView, error) {
	store := externals.NewStore(s.store)
	st, err := store.Ensure(id)
	if err != nil {
		return ExternalView{}, err
	}
	job, err := store.Job(id)
	if err != nil {
		return ExternalView{}, err
	}
	s.externalMu.Lock()
	running := s.externalJobs[id] != nil
	if job.Status == "running" && !running {
		job.Status = "interrupted"
		job.Error = "应用已重启，探索任务未完成，可重新开始"
		job.FinishedAt = externals.Now()
		err = store.SaveJob(job)
	}
	s.externalMu.Unlock()
	if err != nil {
		return ExternalView{}, err
	}
	report, err := store.Discovery(id)
	if err != nil {
		return ExternalView{}, err
	}
	if report != nil {
		d, err := s.store.Load()
		if err != nil {
			return ExternalView{}, err
		}
		r, err := d.Resource(id)
		if err != nil {
			return ExternalView{}, err
		}
		if report.Binding != catalog.ResourceBinding(r, d) {
			report.Status = "stale"
			report.Summary = "连接已改变，请重新探索并核对接口"
		}
	}
	inv, err := store.Invocations(id)
	return ExternalView{st, job, report, inv}, err
}
func (s *Service) SaveExternalOperation(id string, revision uint64, op externals.Definition, reason string) (externals.State, error) {
	return externals.NewStore(s.store).SaveOperation(id, revision, op, reason)
}
func (s *Service) SaveExternalPermissions(id string, revision uint64, p externals.PermissionPolicy, grants []externals.Grant, reason string) (externals.State, error) {
	return externals.NewStore(s.store).SavePermissions(id, revision, p, grants, reason)
}
func (s *Service) SaveExternalRisk(id string, revision uint64, risk externals.RiskPolicy, reason string) (externals.State, error) {
	return externals.NewStore(s.store).SaveRisk(id, revision, risk, reason)
}
func (s *Service) SimulateExternalCall(op resources.Operation, caller externals.Caller) (resources.Preview, error) {
	e := resources.New(s.store, caller.HostID, "", nil)
	e.SessionID = caller.SessionID
	if caller.Local {
		e.HostID = ""
	} else if e.HostID == "" {
		return resources.Preview{}, errors.New("模拟 SSH 调用需要指定主机")
	}
	p, err := e.Preview(op)
	p.Token = ""
	return p, err
}
func (s *Service) PreviewExternalCall(op resources.Operation) (resources.Preview, error) {
	d, err := s.store.Load()
	if err != nil {
		return resources.Preview{}, err
	}
	r, err := d.Resource(op.ResourceID)
	if err != nil {
		return resources.Preview{}, err
	}
	helper := ""
	if r.Route == "ssh" {
		helper, err = cliPath()
		if err != nil {
			return resources.Preview{}, err
		}
	}
	// Desktop executes as the local subject. A tunnel host is transport only.
	e := resources.New(s.store, "", helper, nil)
	p, err := e.Preview(op)
	if err != nil {
		return p, err
	}
	s.externalMu.Lock()
	defer s.externalMu.Unlock()
	if s.externalCalls == nil {
		s.externalCalls = map[string]pendingExternalCall{}
	}
	for k, v := range s.externalCalls {
		if time.Now().After(v.Expires) {
			delete(s.externalCalls, k)
		}
	}
	if len(s.externalCalls) >= 100 {
		return p, errors.New("待确认请求过多")
	}
	s.externalCalls[p.Token] = pendingExternalCall{e, time.Now().Add(5 * time.Minute)}
	return p, nil
}
func (s *Service) InvokeExternalCall(token string) (string, error) {
	s.externalMu.Lock()
	p, ok := s.externalCalls[token]
	delete(s.externalCalls, token)
	s.externalMu.Unlock()
	if !ok || time.Now().After(p.Expires) {
		return "", errors.New("确认令牌已过期或已经使用")
	}
	return p.Executor.ExecutePreview(s.ctx, token)
}
func (s *Service) StartResourceDiscovery(id string) (externals.DiscoveryJob, error) {
	store := externals.NewStore(s.store)
	if _, err := store.Ensure(id); err != nil {
		return externals.DiscoveryJob{}, err
	}
	s.externalMu.Lock()
	defer s.externalMu.Unlock()
	if s.externalJobs == nil {
		s.externalJobs = map[string]context.CancelFunc{}
	}
	if s.externalJobs[id] != nil {
		return store.Job(id)
	}
	ctx, cancel := context.WithCancel(s.ctx)
	job := externals.DiscoveryJob{ID: catalog.NewID(), ConnectionID: id, Status: "running", StartedAt: externals.Now()}
	if err := store.SaveJob(job); err != nil {
		cancel()
		return job, err
	}
	s.externalJobs[id] = cancel
	go func() {
		report, err := s.exploreResource(ctx, id)
		s.externalMu.Lock()
		defer s.externalMu.Unlock()
		delete(s.externalJobs, id)
		job.FinishedAt = externals.Now()
		if ctx.Err() != nil {
			job.Status = "cancelled"
		} else if err != nil {
			job.Status = "failed"
			job.Error = err.Error()
		} else {
			job.Status = report.Status
		}
		cancel()
		_ = store.SaveJob(job)
	}()
	return job, nil
}
func (s *Service) CancelResourceDiscovery(id string) error {
	s.externalMu.Lock()
	defer s.externalMu.Unlock()
	if cancel := s.externalJobs[id]; cancel != nil {
		cancel()
	}
	return nil
}
func (s *Service) CopyExternalDefinitions(source, target string, revision uint64, reason string) (externals.State, error) {
	return externals.NewStore(s.store).CopyDefinitions(source, target, revision, reason)
}
