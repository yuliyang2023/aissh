// Package monitorplan runs bounded, declarative data plans on native SSH captures.
// It never executes commands or component code.
package monitorplan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"time"
)

const MaxRaw = 256 << 10
const MaxDataset = 64 << 10
const MaxRows = 1000

type Source struct {
	ID        string `json:"id"`
	Command   string `json:"command"`
	TimeoutMS int    `json:"timeoutMs"`
	Required  bool   `json:"required"`
}
type Capture struct {
	ID             string    `json:"captureId"`
	SourceID       string    `json:"sourceId"`
	SessionID      string    `json:"sessionId"`
	Generation     uint64    `json:"sessionGeneration"`
	CommandHash    string    `json:"commandHash"`
	SourceRevision uint64    `json:"sourceRevision"`
	Sequence       uint64    `json:"sequence"`
	StartedAt      time.Time `json:"startedAt"`
	FinishedAt     time.Time `json:"finishedAt"`
	DurationMS     int64     `json:"durationMs"`
	ExitCode       *int      `json:"exitCode"`
	Transport      string    `json:"transport"`
	Streams        string    `json:"streams"`
	Stdout         string    `json:"stdout"`
	Stderr         *string   `json:"stderr"`
	Status         string    `json:"status"`
	Complete       bool      `json:"complete"`
}

func (c Capture) Good() bool {
	return c.Status == "ok" && c.Complete && c.ExitCode != nil && *c.ExitCode == 0 && len(c.Stdout) <= MaxRaw
}
func Hash(command string) string {
	s := sha256.Sum256([]byte(command))
	return hex.EncodeToString(s[:])
}

type SourceRef struct {
	SourceID  string `json:"sourceId"`
	CaptureID string `json:"captureId"`
}

// Request is the three-phase AI API: probe supplies sources, map supplies refs
// and a data plan, create/update supplies the returned mapId and bindings.
type Request struct {
	ProtocolVersion int         `json:"protocolVersion"`
	Sources         []Source    `json:"sources,omitempty"`
	Refs            []SourceRef `json:"refs,omitempty"`
	DataPlan        *Plan       `json:"dataPlan,omitempty"`
	MapID           string      `json:"mapId,omitempty"`
	Bindings        []Binding   `json:"bindings,omitempty"`
}
type Column struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Unit string `json:"unit,omitempty"`
}
type Parser struct {
	ID             string   `json:"id"`
	SourceID       string   `json:"sourceId"`
	Kind           string   `json:"kind"`
	Columns        []Column `json:"columns"`
	Pattern        string   `json:"pattern,omitempty"`
	Delimiter      string   `json:"delimiter,omitempty"`
	SkipBlankLines bool     `json:"skipBlankLines,omitempty"`
	Header         bool     `json:"header,omitempty"`
	OnUnmatched    string   `json:"onUnmatched,omitempty"`
	Path           []string `json:"path,omitempty"`
}
type Pair struct {
	From string `json:"from"`
	To   string `json:"to"`
}
type Step struct {
	ID         string   `json:"id"`
	Op         string   `json:"op"`
	Input      string   `json:"input"`
	Other      string   `json:"other,omitempty"`
	Field      string   `json:"field,omitempty"`
	Test       string   `json:"test,omitempty"`
	Values     []string `json:"values,omitempty"`
	Min        *float64 `json:"min,omitempty"`
	Max        *float64 `json:"max,omitempty"`
	Pattern    string   `json:"pattern,omitempty"`
	Fields     []string `json:"fields,omitempty"`
	Names      []Pair   `json:"names,omitempty"`
	From       string   `json:"from,omitempty"`
	To         string   `json:"to,omitempty"`
	Value      *float64 `json:"value,omitempty"`
	Key        string   `json:"key,omitempty"`
	OtherKey   string   `json:"otherKey,omitempty"`
	MaxSkewMS  int      `json:"maxSkewMs,omitempty"`
	Descending bool     `json:"descending,omitempty"`
	Limit      int      `json:"limit,omitempty"`
	Window     int      `json:"window,omitempty"`
	Semantics  string   `json:"semantics,omitempty"`
}
type Output struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Input   string `json:"input"`
	Unit    string `json:"unit,omitempty"`
	History bool   `json:"history,omitempty"`
}
type Plan struct {
	Parsers []Parser `json:"parsers"`
	Steps   []Step   `json:"steps"`
	Outputs []Output `json:"outputs"`
}
type Binding struct {
	Prop     string `json:"prop"`
	Kind     string `json:"kind"`
	OutputID string `json:"outputId"`
}
type Definition struct {
	ProtocolVersion int       `json:"protocolVersion"`
	Sources         []Source  `json:"sources"`
	DataPlan        Plan      `json:"dataPlan"`
	Bindings        []Binding `json:"bindings"`
}
type Diagnostic struct {
	Stage    string `json:"stage"`
	SourceID string `json:"sourceId,omitempty"`
	NodeID   string `json:"nodeId,omitempty"`
	Message  string `json:"message"`
}
type Metric struct {
	Value      any        `json:"value"`
	Unit       string     `json:"unit"`
	Quality    string     `json:"quality"`
	ObservedAt *time.Time `json:"observedAt"`
	SourceIDs  []string   `json:"sourceIds"`
	Reason     string     `json:"reason,omitempty"`
}
type Table struct {
	Columns     []Column         `json:"columns"`
	Rows        []map[string]any `json:"rows"`
	Quality     string           `json:"quality"`
	ObservedAt  *time.Time       `json:"observedAt"`
	SourceIDs   []string         `json:"sourceIds"`
	TotalRows   *int             `json:"totalRows"`
	ShownRows   int              `json:"shownRows"`
	OmittedRows *int             `json:"omittedRows"`
	Reason      string           `json:"reason,omitempty"`
}
type Dataset struct {
	DefinitionRevision uint64            `json:"definitionRevision"`
	DataRevision       uint64            `json:"dataRevision"`
	Sequence           uint64            `json:"sequence"`
	Metrics            map[string]Metric `json:"metrics"`
	Tables             map[string]Table  `json:"tables"`
	Diagnostics        []Diagnostic      `json:"diagnostics"`
}

var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]{0,63}$`)

func ValidID(s string) bool {
	return identifier.MatchString(s) && s != "__proto__" && s != "prototype" && s != "constructor"
}
func ValidateSources(sources []Source) error {
	if len(sources) == 0 || len(sources) > 8 {
		return fmt.Errorf("需要 1–8 个数据源")
	}
	seen := map[string]bool{}
	required := false
	for _, s := range sources {
		if !ValidID(s.ID) || seen[s.ID] || s.Command == "" || len(s.Command) > 32768 || s.TimeoutMS < 1000 || s.TimeoutMS > 30000 {
			return fmt.Errorf("数据源 %q 无效（超时范围 1000–30000 ms）", s.ID)
		}
		seen[s.ID] = true
		required = required || s.Required
	}
	if !required {
		return fmt.Errorf("至少一个核心数据源必须为 required")
	}
	return nil
}
func (d Definition) Validate() error {
	if d.ProtocolVersion != 2 {
		return fmt.Errorf("数据协议必须为 2")
	}
	if err := ValidateSources(d.Sources); err != nil {
		return err
	}
	if err := d.DataPlan.Validate(d.Sources); err != nil {
		return err
	}
	return ValidateBindings(d.DataPlan, d.Bindings)
}
func ValidateBindings(p Plan, bindings []Binding) error {
	if len(bindings) == 0 || len(bindings) > 50 {
		return fmt.Errorf("需要 1–50 个数据绑定")
	}
	outputs := map[string]Output{}
	for _, o := range p.Outputs {
		outputs[o.ID] = o
	}
	seen := map[string]bool{}
	for _, b := range bindings {
		if !ValidID(b.Prop) || seen[b.Prop] || outputs[b.OutputID].Kind != b.Kind || b.Kind == "" {
			return fmt.Errorf("无效绑定 %q → %q", b.Prop, b.OutputID)
		}
		seen[b.Prop] = true
	}
	return nil
}
func (d Dataset) Bind(bindings []Binding) map[string]any {
	out := map[string]any{}
	for _, b := range bindings {
		if b.Kind == "metric" {
			out[b.Prop] = d.Metrics[b.OutputID]
		} else {
			out[b.Prop] = d.Tables[b.OutputID]
		}
	}
	return out
}
func (p Plan) Fingerprint() string { b, _ := json.Marshal(p); return Hash(string(b)) }
