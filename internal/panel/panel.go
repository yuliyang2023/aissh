package panel

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"git.example.com/ai/wrd-ai-cli/internal/monitorplan"
	"git.example.com/ai/wrd-ai-cli/internal/widgets"
	"strings"
	"time"
)

type Kind string

const (
	KindAI      Kind = "ai"
	KindMonitor Kind = "monitor"
	KindLog     Kind = "log"
	KindTable   Kind = "table"
	KindReport  Kind = "report"
	KindShell   Kind = "shell"
)

type Origin string

const (
	OriginSystem Origin = "system"
	OriginUser   Origin = "user"
	OriginAI     Origin = "ai"
)

type Panel struct {
	Pipeline         *monitorplan.Definition
	PipelineEngine   *monitorplan.Engine
	PipelineCaptures map[string]monitorplan.Capture

	Widget             *widgets.Artifact
	DataContract       *widgets.Contract
	WidgetData         widgets.Data
	DefinitionRevision uint64
	BindingID          string
	RunID              uint64

	// Purpose separates display collectors from independently managed alert tasks.
	Purpose string
	Alert   *AlertCondition
	// SampleContent is only written by a completed collector. AI append/content
	// operations must not be allowed to fabricate alert measurements.
	SampleContent  string
	SourceRevision uint64
	ID             string
	Kind           Kind
	Origin         Origin
	Title          string
	Command        string
	Interval       time.Duration
	Content        string
	Status         string
	Pinned         bool
	Unread         bool
	AllowAIRead    bool
	Target         string
	LastUpdated    time.Time
	LastError      string
	RefreshCount   uint64
}

func (p *Panel) IsAlert() bool { return p.Purpose == "alert" }

type Manager struct {
	panels []*Panel
	active int
	nextID uint64
}

func NewManager(target string) *Manager {
	m := &Manager{}
	m.Open(Panel{ID: "system", Kind: KindMonitor, Origin: OriginSystem, Title: "系统", Command: BuiltinOverviewCommand, Interval: 2 * time.Second, Pinned: true, AllowAIRead: true, Target: target})
	return m
}

func (m *Manager) Panels() []*Panel { return m.panels }

func (m *Manager) CountPurpose(alert bool) int {
	count := 0
	for _, p := range m.panels {
		if p.IsAlert() == alert {
			count++
		}
	}
	return count
}

func (m *Manager) Active() *Panel {
	if len(m.panels) == 0 {
		return nil
	}
	if m.active >= len(m.panels) {
		m.active = len(m.panels) - 1
	}
	return m.panels[m.active]
}

func (m *Manager) ActiveIndex() int { return m.active }

func (m *Manager) Activate(id string) bool {
	for index, item := range m.panels {
		if item.ID == id {
			m.active = index
			item.Unread = false
			return true
		}
	}
	return false
}

func (m *Manager) Next() {
	if len(m.panels) == 0 {
		return
	}
	m.active = (m.active + 1) % len(m.panels)
	m.panels[m.active].Unread = false
}

func (m *Manager) Previous() {
	if len(m.panels) == 0 {
		return
	}
	m.active = (m.active + len(m.panels) - 1) % len(m.panels)
	m.panels[m.active].Unread = false
}

func (m *Manager) Open(item Panel) *Panel {
	if item.Purpose == "" {
		item.Purpose = "realtime"
	}
	if item.ID != "" {
		if existing := m.ByID(item.ID); existing != nil {
			return existing
		}
	}
	if item.ID == "" {
		m.nextID++
		item.ID = fmt.Sprintf("panel-%d", m.nextID)
	}
	if item.Interval <= 0 && (item.Kind == KindMonitor || item.Kind == KindTable) {
		item.Interval = 2 * time.Second
	}
	if item.BindingID == "" {
		var token [16]byte
		_, _ = rand.Read(token[:])
		item.BindingID = hex.EncodeToString(token[:])
	}
	if item.DefinitionRevision == 0 {
		item.DefinitionRevision = 1
	}
	copy := item
	m.panels = append(m.panels, &copy)
	return &copy
}

func (m *Manager) ByID(id string) *Panel {
	for _, item := range m.panels {
		if item.ID == id {
			return item
		}
	}
	return nil
}

func (m *Manager) Close(id string, requester Origin) error {
	for index, item := range m.panels {
		if item.ID != id {
			continue
		}
		if item.Pinned {
			return fmt.Errorf("panel %s is pinned", id)
		}
		if requester == OriginAI && item.Origin != OriginAI {
			return fmt.Errorf("AI cannot close a user panel")
		}
		m.panels = append(m.panels[:index], m.panels[index+1:]...)
		if m.active >= len(m.panels) {
			m.active = len(m.panels) - 1
		}
		if m.active < 0 {
			m.active = 0
		}
		return nil
	}
	return fmt.Errorf("panel %s not found", id)
}

func (m *Manager) UpdateContent(id, content, status string, foreground bool) {
	item := m.ByID(id)
	if item == nil {
		return
	}
	item.Content = strings.TrimSpace(content)
	item.SampleContent = item.Content
	item.Status = status
	item.LastUpdated = time.Now()
	item.RefreshCount++
	item.LastError = ""
	if item.DataContract != nil {
		item.WidgetData.BindingID = item.BindingID
		item.WidgetData.SourceRevision = item.SourceRevision
		item.WidgetData.IntervalMS = item.Interval.Milliseconds()
		item.WidgetData = widgets.Record(item.WidgetData, *item.DataContract, item.SampleContent, nil, item.LastUpdated)
	}
	if !foreground && m.Active() != item {
		item.Unread = true
	}
}

func (m *Manager) SetError(id string, err error) {
	item := m.ByID(id)
	if item == nil || err == nil {
		return
	}
	item.LastError = err.Error()
	item.Status = "采集失败"
	item.LastUpdated = time.Now()
	if item.DataContract != nil {
		item.WidgetData.BindingID = item.BindingID
		item.WidgetData.SourceRevision = item.SourceRevision
		item.WidgetData.IntervalMS = item.Interval.Milliseconds()
		item.WidgetData = widgets.Record(item.WidgetData, *item.DataContract, "", err, item.LastUpdated)
	}
	if m.Active() != item {
		item.Unread = true
	}
}

const BuiltinOverviewCommand = `LC_ALL=C; printf 'HOST='; hostname 2>/dev/null; printf 'UPTIME='; uptime 2>/dev/null; printf 'CORES='; (getconf _NPROCESSORS_ONLN 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null); printf 'LOAD='; (cut -d' ' -f1-3 /proc/loadavg 2>/dev/null || sysctl -n vm.loadavg 2>/dev/null); printf 'MEM='; (free -m 2>/dev/null | awk 'NR==2 {printf "%s/%s MB (%.1f%%)", $3,$2,$3*100/$2}' || true); printf '\nDISK='; df -hP / 2>/dev/null | awk 'NR==2 {printf "%s/%s (%s)\n", $3,$2,$5}'; printf 'PROCS='; (ps -e 2>/dev/null | wc -l | tr -d ' '); printf '\n'`

const BuiltinProcessCommand = `LC_ALL=C; ps -eo pid,user,comm,%cpu,%mem --sort=-%cpu 2>/dev/null | head -n 18 || ps aux 2>/dev/null | head -n 18`

const BuiltinDiskCommand = `LC_ALL=C; df -hP 2>/dev/null; printf '\nTop directories:\n'; du -x -d 1 / 2>/dev/null | sort -nr | head -n 12`

const BuiltinNetworkCommand = `LC_ALL=C; printf 'Interfaces:\n'; (ip -s link 2>/dev/null || ifconfig 2>/dev/null); printf '\nConnections:\n'; (ss -s 2>/dev/null || netstat -an 2>/dev/null | head -n 25)`

const BuiltinServiceCommand = `LC_ALL=C; if command -v systemctl >/dev/null 2>&1; then systemctl --failed --no-pager 2>/dev/null; else service --status-all 2>/dev/null | head -n 30; fi`
