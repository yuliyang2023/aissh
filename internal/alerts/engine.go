// Package alerts evaluates local monitoring data without executing commands or
// sending it to an AI provider. The caller serializes access to Engine.
package alerts

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/credentialui"
	"git.example.com/ai/wrd-ai-cli/internal/panel"
)

type Rule struct {
	Condition      panel.AlertCondition `json:"-"`
	SourceRevision uint64               `json:"-"`
	ID             string               `json:"id"`
	Name           string               `json:"name"`
	Metric         string               `json:"metric"`
	Enabled        bool                 `json:"enabled"`
	Threshold      float64              `json:"threshold"`
	Severity       string               `json:"severity"`
	PanelID        string               `json:"panelId"`
	Keyword        string               `json:"keyword"`
}

type Policy struct {
	Enabled         bool   `json:"enabled"`
	PetEnabled      bool   `json:"petEnabled"`
	PetStyle        string `json:"petStyle"`
	ConfirmSeconds  int    `json:"confirmSeconds"`
	RecoverySeconds int    `json:"recoverySeconds"`
	RepeatMinutes   int    `json:"repeatMinutes"`
	StaleSeconds    int    `json:"staleSeconds"`
	NotifyRecovery  bool   `json:"notifyRecovery"`
	Rules           []Rule `json:"rules"`
}

func DefaultPolicy() Policy {
	return Policy{Enabled: true, PetStyle: "classic", ConfirmSeconds: 10, RecoverySeconds: 10, RepeatMinutes: 5, StaleSeconds: 30, NotifyRecovery: true, Rules: []Rule{
		{ID: "collection", Name: "告警任务采集失败", Metric: "collection", Enabled: true, Severity: "warning"},
		{ID: "stale", Name: "告警任务数据过期", Metric: "stale", Enabled: true, Severity: "warning"},
		{ID: "offline", Name: "告警任务所在 SSH 断线", Metric: "offline", Enabled: true, Severity: "critical"},
	}}
}

func (p Policy) Validate() error {
	if p.PetStyle != "" && p.PetStyle != "classic" && p.PetStyle != "hacker" && p.PetStyle != "space" {
		return errors.New("未知的桌宠样式")
	}
	if p.ConfirmSeconds < 0 || p.ConfirmSeconds > 3600 || p.RecoverySeconds < 0 || p.RecoverySeconds > 3600 || p.RepeatMinutes < 1 || p.RepeatMinutes > 1440 || p.StaleSeconds < 15 || p.StaleSeconds > 3600 {
		return errors.New("确认和恢复等待须为 0–3600 秒，重复提醒为 1–1440 分钟，数据过期为 15–3600 秒")
	}
	if len(p.Rules) > 32 {
		return errors.New("最多配置 32 条提醒规则")
	}
	ids := map[string]bool{}
	for _, r := range p.Rules {
		if r.ID == "" || strings.HasPrefix(r.ID, "panel:") || len(r.ID) > 80 || ids[r.ID] || strings.TrimSpace(r.Name) == "" || len(r.Name) > 160 || (r.Severity != "warning" && r.Severity != "critical") {
			return errors.New("提醒规则的标识、名称或级别无效")
		}
		ids[r.ID] = true
		if math.IsNaN(r.Threshold) || math.IsInf(r.Threshold, 0) {
			return errors.New("阈值无效")
		}
		switch r.Metric {
		case "memory", "disk":
			if r.Threshold < 5 || r.Threshold > 100 {
				return errors.New("使用率阈值须为 5–100%")
			}
		case "load":
			if r.Threshold <= 0 || r.Threshold > 100 {
				return errors.New("每核负载阈值须大于 0 且不超过 100")
			}
		case "collection", "stale", "offline":
		case "keyword":
			if strings.TrimSpace(r.PanelID) == "" || len(r.PanelID) > 100 || strings.TrimSpace(r.Keyword) == "" || len(r.Keyword) > 200 {
				return errors.New("关键词规则需要监控 ID 和关键词（最多 200 字节）")
			}
		default:
			return errors.New("未知的提醒指标")
		}
	}
	return nil
}

type Session struct {
	ID, Name, Status string
	CreatedAt        time.Time
	Panels           []credentialui.MonitorPanel
}

type Incident struct {
	ID           string     `json:"id"`
	SessionID    string     `json:"sessionId"`
	SessionName  string     `json:"sessionName"`
	RuleID       string     `json:"ruleId"`
	Title        string     `json:"title"`
	Detail       string     `json:"detail"`
	Severity     string     `json:"severity"`
	Status       string     `json:"status"` // firing, recovered, stopped
	OpenedAt     time.Time  `json:"openedAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	ClosedAt     *time.Time `json:"closedAt,omitempty"`
	Acknowledged bool       `json:"acknowledged"`
}

type Notice struct {
	Sequence uint64    `json:"sequence"`
	Incident Incident  `json:"incident"`
	Kind     string    `json:"kind"`
	Time     time.Time `json:"time"`
}

type State struct {
	Monitors     []MonitorSummary `json:"monitors"`
	Policy       Policy           `json:"policy"`
	Incidents    []Incident       `json:"incidents"`
	Notices      []Notice         `json:"notices"`
	SnoozedUntil time.Time        `json:"snoozedUntil"`
	LiveSessions int              `json:"liveSessions"`
	PetRunning   bool             `json:"petRunning"`
	Error        string           `json:"error"`
}

type MonitorSummary struct {
	Command     string                `json:"command"`
	Origin      string                `json:"origin"`
	UpdatedAt   string                `json:"updatedAt"`
	Error       string                `json:"error"`
	SessionID   string                `json:"sessionId"`
	SessionName string                `json:"sessionName"`
	PanelID     string                `json:"panelId"`
	Title       string                `json:"title"`
	IntervalMS  int64                 `json:"intervalMs"`
	Alert       *panel.AlertCondition `json:"alert,omitempty"`
	Status      string                `json:"status"`
}

type tracker struct {
	sample                        string
	pending, recovering, notified time.Time
	incident                      *Incident
	rule                          Rule
}

type Engine struct {
	Policy       Policy
	SnoozedUntil time.Time
	items        map[string]*tracker
	history      []Incident
	notices      []Notice
	sequence     uint64
	live         int
	lastTick     time.Time
	monitors     []MonitorSummary
}

func New(policy Policy) *Engine { return &Engine{Policy: policy, items: map[string]*tracker{}} }

func (e *Engine) Tick(now time.Time, sessions []Session) {
	if !e.lastTick.IsZero() && now.Sub(e.lastTick) > time.Duration(e.Policy.StaleSeconds)*time.Second {
		// Sleep or an interrupted evaluator is not evidence of a continuous
		// failure (or recovery). Require a new confirmation window after it.
		for _, t := range e.items {
			t.pending, t.recovering = time.Time{}, time.Time{}
		}
	}
	e.lastTick = now
	seen := map[string]bool{}
	e.live = 0
	e.monitors = []MonitorSummary{}
	for _, s := range sessions {
		if s.Status == "running" || s.Status == "starting" {
			e.live++
		}
		rules := append([]Rule{}, e.Policy.Rules...)
		for _, p := range s.Panels {
			m := MonitorSummary{SessionID: s.ID, SessionName: s.Name, PanelID: p.ID, Title: p.Title, IntervalMS: max(1000, p.IntervalMS), Status: "watching",
				Command: p.Command, Origin: p.Origin, UpdatedAt: p.UpdatedAt, Error: p.Error}
			if p.Alert != nil {
				condition := *p.Alert
				m.Alert = &condition
				if condition.Enabled && condition.Validate() == nil {
					r := Rule{ID: "panel:" + p.ID, Name: p.Title + "异常", Metric: "custom", Enabled: true, Severity: condition.Severity, PanelID: p.ID, Condition: condition, SourceRevision: p.SourceRevision}
					rules = append(rules, r)
					if !observe(now, e.Policy, r, s).known {
						m.Status = "unknown"
					}
				}
			}
			updated, _ := time.Parse(time.RFC3339Nano, p.UpdatedAt)
			switch {
			case s.Status != "running" && s.Status != "starting":
				m.Status = "stopped"
			case p.Collecting:
				m.Status = "collecting"
			case p.Error != "":
				m.Status = "error"
			case updated.IsZero():
				m.Status = "waiting"
			case now.Sub(updated) > max(time.Duration(e.Policy.StaleSeconds)*time.Second, time.Duration(p.IntervalMS)*3*time.Millisecond):
				m.Status = "stale"
			}
			e.monitors = append(e.monitors, m)
		}
		if !e.Policy.Enabled {
			continue
		}
		for _, r := range rules {
			if !r.Enabled {
				continue
			}
			key := s.ID + "\x00" + r.ID
			seen[key] = true
			t := e.items[key]
			if t != nil && t.rule != r {
				e.stop(now, t)
				delete(e.items, key)
				t = nil
			}
			if t == nil {
				t = &tracker{rule: r}
				e.items[key] = t
			}
			o := observe(now, e.Policy, r, s)
			confirmSeconds, recoverySeconds := e.Policy.ConfirmSeconds, e.Policy.RecoverySeconds
			if r.Metric == "custom" {
				confirmSeconds, recoverySeconds = r.Condition.ForSeconds, r.Condition.RecoverySeconds
			}
			if !o.known {
				t.pending, t.recovering = time.Time{}, time.Time{}
				continue
			}
			// A repeated bridge snapshot is not another measurement. Timers for
			// numeric/keyword rules only advance when a fresh sample arrives.
			if o.sample != t.sample {
				t.sample = o.sample
				if o.failing {
					t.recovering = time.Time{}
					if t.pending.IsZero() {
						t.pending = now
					}
					if t.incident == nil && now.Sub(t.pending) >= time.Duration(confirmSeconds)*time.Second {
						e.sequence++
						t.incident = &Incident{ID: fmt.Sprintf("alert-%d", e.sequence), SessionID: s.ID, SessionName: s.Name, RuleID: r.ID, Title: r.Name, Detail: o.detail, Severity: r.Severity, Status: "firing", OpenedAt: now, UpdatedAt: now}
					}
					if t.incident != nil {
						t.incident.Detail, t.incident.UpdatedAt = o.detail, now
					}
				} else {
					t.pending = time.Time{}
					if o.recovered && t.incident != nil {
						if t.recovering.IsZero() {
							t.recovering = now
						}
						if now.Sub(t.recovering) >= time.Duration(recoverySeconds)*time.Second {
							t.incident.Status, t.incident.ClosedAt, t.incident.UpdatedAt = "recovered", &now, now
							if e.Policy.NotifyRecovery && !t.notified.IsZero() {
								e.notify(now, *t.incident, "recovered")
							}
							e.archive(*t.incident)
							t.incident, t.notified, t.recovering = nil, time.Time{}, time.Time{}
						}
					} else {
						t.recovering = time.Time{}
					}
				}
			}
			if t.incident != nil && o.failing && !t.incident.Acknowledged && (t.notified.IsZero() || now.Sub(t.notified) >= time.Duration(e.Policy.RepeatMinutes)*time.Minute) {
				kind := "firing"
				if !t.notified.IsZero() {
					kind = "repeat"
				}
				if e.notify(now, *t.incident, kind) {
					t.notified = now
				}
			}
		}
	}
	for key, t := range e.items {
		if !seen[key] {
			e.stop(now, t)
			delete(e.items, key)
		}
	}
	sort.Slice(e.monitors, func(i, j int) bool {
		if e.monitors[i].SessionID != e.monitors[j].SessionID {
			return e.monitors[i].SessionID < e.monitors[j].SessionID
		}
		return e.monitors[i].PanelID < e.monitors[j].PanelID
	})
}

func (e *Engine) stop(now time.Time, t *tracker) {
	if t.incident != nil {
		t.incident.Status, t.incident.ClosedAt, t.incident.UpdatedAt = "stopped", &now, now
		e.archive(*t.incident)
	}
}
func (e *Engine) archive(i Incident) {
	e.history = append(e.history, i)
	if len(e.history) > 100 {
		e.history = e.history[len(e.history)-100:]
	}
}
func (e *Engine) notify(now time.Time, i Incident, kind string) bool {
	if now.Before(e.SnoozedUntil) {
		return false
	}
	e.sequence++
	e.notices = append(e.notices, Notice{Sequence: e.sequence, Incident: i, Kind: kind, Time: now})
	if len(e.notices) > 30 {
		e.notices = e.notices[len(e.notices)-30:]
	}
	return true
}
func (e *Engine) Acknowledge(id string) error {
	for _, t := range e.items {
		if t.incident != nil && t.incident.ID == id {
			t.incident.Acknowledged = true
			return nil
		}
	}
	return errors.New("该告警已结束，请刷新列表")
}
func (e *Engine) Snapshot() State {
	result := State{Policy: e.Policy, SnoozedUntil: e.SnoozedUntil, LiveSessions: e.live, Incidents: append([]Incident{}, e.history...), Notices: append([]Notice{}, e.notices...)}
	result.Policy.Rules = append([]Rule{}, e.Policy.Rules...)
	result.Monitors = append([]MonitorSummary{}, e.monitors...)
	for i := range result.Monitors {
		if result.Monitors[i].Alert != nil {
			copy := *result.Monitors[i].Alert
			result.Monitors[i].Alert = &copy
		}
	}
	for _, t := range e.items {
		if t.incident != nil {
			result.Incidents = append(result.Incidents, *t.incident)
		}
	}
	sort.Slice(result.Incidents, func(i, j int) bool {
		if result.Incidents[i].UpdatedAt.Equal(result.Incidents[j].UpdatedAt) {
			return result.Incidents[i].ID < result.Incidents[j].ID
		}
		return result.Incidents[i].UpdatedAt.After(result.Incidents[j].UpdatedAt)
	})
	return result
}

type observation struct {
	known, failing, recovered bool
	sample, detail            string
}

var percent = regexp.MustCompile(`\(([0-9]+(?:\.[0-9]+)?)%\)`)

func observe(now time.Time, p Policy, r Rule, s Session) observation {
	boolean := func(fail bool, sample, detail string) observation {
		return observation{true, fail, !fail, sample, detail}
	}
	live := s.Status == "running" || s.Status == "starting"
	if r.Metric == "offline" {
		return boolean(!live, now.String(), "SSH 会话已结束，监控已停止；请检查连接并重新连接。")
	}
	if !live {
		return observation{}
	}
	var system *credentialui.MonitorPanel
	for i := range s.Panels {
		if s.Panels[i].ID == "system" {
			system = &s.Panels[i]
		}
	}
	if r.Metric == "collection" {
		for _, panel := range s.Panels {
			if panel.Error != "" {
				return boolean(true, now.String(), panel.Title+"采集失败，请在告警监控中查看任务详情。")
			}
		}
		return boolean(false, now.String(), "")
	}
	if r.Metric == "stale" {
		if len(s.Panels) == 0 {
			return boolean(now.Sub(s.CreatedAt) > time.Duration(p.StaleSeconds)*time.Second, now.String(), "尚未收到监控数据，请检查 SSH 连接。")
		}
		for _, panel := range s.Panels {
			updated, _ := time.Parse(time.RFC3339Nano, panel.UpdatedAt)
			if updated.IsZero() {
				updated = s.CreatedAt
			}
			if now.Sub(updated) > max(time.Duration(p.StaleSeconds)*time.Second, time.Duration(panel.IntervalMS)*3*time.Millisecond) {
				return boolean(true, now.String(), panel.Title+"数据已过期；SSH 忙碌、采集暂停或连接异常均可能导致此状态。")
			}
		}
		return boolean(false, now.String(), "")
	}
	if r.Metric == "keyword" || r.Metric == "custom" {
		system = nil
		for i := range s.Panels {
			if s.Panels[i].ID == r.PanelID {
				system = &s.Panels[i]
			}
		}
	}
	if system == nil || system.Error != "" {
		return observation{}
	}
	updated, err := time.Parse(time.RFC3339Nano, system.UpdatedAt)
	if err != nil || updated.IsZero() || now.Sub(updated) > max(time.Duration(p.StaleSeconds)*time.Second, time.Duration(system.IntervalMS)*3*time.Millisecond) {
		return observation{}
	}
	if r.Metric == "custom" {
		content := strings.TrimSpace(system.SampleContent)
		if content == "" || r.Condition.Validate() != nil {
			return observation{}
		}
		c := r.Condition
		re, _ := regexp.Compile(c.Pattern)
		if c.Kind == "match" || c.Kind == "not_match" {
			fail := re.MatchString(content)
			if c.Kind == "not_match" {
				fail = !fail
			}
			return boolean(fail, system.UpdatedAt, system.Title+"的采集结果符合配置的异常条件，请查看告警监控中的任务配置。")
		}
		match := re.FindStringSubmatch(content)
		if len(match) != 2 {
			return observation{}
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(match[1]), 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			return observation{}
		}
		fail, recovered := value >= c.Threshold, value <= c.RecoveryThreshold
		if c.Kind == "below" {
			fail, recovered = value <= c.Threshold, value >= c.RecoveryThreshold
		}
		return observation{true, fail, recovered, system.UpdatedAt, fmt.Sprintf("采集值 %g，异常阈值 %g，恢复阈值 %g。", value, c.Threshold, c.RecoveryThreshold)}
	}
	if r.Metric == "keyword" {
		return boolean(strings.Contains(strings.ToLower(system.Content), strings.ToLower(r.Keyword)), system.UpdatedAt, system.Title+"命中配置的异常关键词。")
	}
	values := map[string]string{}
	for _, line := range strings.Split(system.Content, "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			values[k] = strings.TrimSpace(v)
		}
	}
	value := 0.0
	recovery := r.Threshold - 5
	unit := "%"
	if r.Metric == "load" {
		fields := strings.Fields(strings.Trim(values["LOAD"], "{} "))
		cores, err := strconv.ParseFloat(values["CORES"], 64)
		if err != nil || cores <= 0 || len(fields) == 0 {
			return observation{}
		}
		load, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			return observation{}
		}
		value, recovery, unit = load/cores, r.Threshold*0.9, " / 核"
	} else {
		key := "MEM"
		if r.Metric == "disk" {
			key = "DISK"
		}
		match := percent.FindStringSubmatch(values[key])
		if len(match) != 2 {
			return observation{}
		}
		value, err = strconv.ParseFloat(match[1], 64)
		if err != nil || value > 100 {
			return observation{}
		}
	}
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return observation{}
	}
	return observation{true, value >= r.Threshold, value <= recovery, system.UpdatedAt, fmt.Sprintf("当前 %.1f%s，提醒阈值 %.1f%s。", value, unit, r.Threshold, unit)}
}
