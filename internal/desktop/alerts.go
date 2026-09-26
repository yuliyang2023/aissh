package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/alerts"
)

func (s *Service) loadAlertPolicy() {
	p := alerts.DefaultPolicy()
	data, err := os.ReadFile(filepath.Join(s.store.Dir, "monitor-alerts.json"))
	if err == nil {
		var saved alerts.Policy
		err = json.Unmarshal(data, &saved)
		if err == nil {
			err = saved.Validate()
		}
		if err == nil {
			p = taskAlertPolicy(saved)
		}
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		s.alertError = "提醒策略读取失败，已使用默认策略；请检查并重新保存。"
	}
	s.alerts = alerts.New(p)
}

func (s *Service) monitorInputs() []alerts.Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	inputs := make([]alerts.Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		session.mu.Lock()
		created, _ := time.Parse(time.RFC3339Nano, session.info.CreatedAt)
		input := alerts.Session{ID: session.id, Name: session.info.Name, Status: session.info.Status, CreatedAt: created}
		if session.workbench != nil {
			input.Panels = session.workbench.AlertMonitors
		}
		// Display-only panels never create incidents, including on disconnect.
		if len(input.Panels) > 0 {
			inputs = append(inputs, input)
		}
		session.mu.Unlock()
	}
	return inputs
}

func (s *Service) tickAlerts(now time.Time) {
	inputs := s.monitorInputs()
	s.alertMu.Lock()
	s.alerts.Tick(now, inputs)
	s.alertMu.Unlock()
}

func (s *Service) runAlerts(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tickAlerts(time.Now())
		}
	}
}

func (s *Service) GetMonitoring() alerts.State {
	s.alertMu.Lock()
	state := s.alerts.Snapshot()
	state.Error = s.alertError
	s.alertMu.Unlock()
	s.petMu.Lock()
	state.PetRunning = s.pet != nil
	s.petMu.Unlock()
	return state
}

func (s *Service) SaveAlertPolicy(p alerts.Policy) error {
	p = taskAlertPolicy(p)
	if err := p.Validate(); err != nil {
		return err
	}
	s.alertMu.Lock()
	p.PetEnabled = s.alerts.Policy.PetEnabled // managed by SetPetEnabled
	err := saveAlertPolicy(s.store.Dir, p)
	if err == nil {
		s.alerts.Policy = p
		s.alertError = ""
	}
	s.alertMu.Unlock()
	if err != nil {
		return err
	}
	s.tickAlerts(time.Now())
	return nil
}

// Retire legacy display-based thresholds while preserving notification settings.
// Content conditions now belong to explicit, independent alert tasks.
func taskAlertPolicy(p alerts.Policy) alerts.Policy {
	rules := make([]alerts.Rule, 0, len(p.Rules))
	for _, r := range p.Rules {
		if r.Metric == "collection" || r.Metric == "stale" || r.Metric == "offline" {
			rules = append(rules, r)
		}
	}
	p.Rules = rules
	return p
}

func saveAlertPolicy(dir string, p alerts.Policy) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".monitor-alerts-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), filepath.Join(dir, "monitor-alerts.json"))
}

func (s *Service) AcknowledgeAlert(id string) error {
	s.alertMu.Lock()
	defer s.alertMu.Unlock()
	return s.alerts.Acknowledge(id)
}

func (s *Service) SnoozeAlerts(minutes int) error {
	if minutes < 0 || minutes > 1440 {
		return errors.New("静音时长须为 0–1440 分钟")
	}
	s.alertMu.Lock()
	defer s.alertMu.Unlock()
	s.alerts.SnoozedUntil = time.Time{}
	if minutes > 0 {
		s.alerts.SnoozedUntil = time.Now().Add(time.Duration(minutes) * time.Minute)
	}
	return nil
}

func (s *Service) OpenMonitorSession(id string) error {
	if id != "" {
		if _, err := s.session(id); err != nil {
			return err
		}
	}
	if s.focusMain != nil {
		s.focusMain()
	}
	if id == "" {
		s.emit("desktop-command", "monitoring")
	} else {
		s.emit("monitor-session", id)
	}
	return nil
}
