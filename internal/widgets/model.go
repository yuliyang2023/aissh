// Package widgets defines the data-only boundary of AI-authored monitor views.
package widgets

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"git.example.com/ai/wrd-ai-cli/internal/monitorplan"
	"regexp"
	"strings"
)

const SDKVersion = 1
const MaxSource = 128 << 10
const MaxBundle = 512 << 10
const MaxSnapshot = 64 << 10
const MaxHistory = 128 << 10
const MaxOutput = 256 << 10

type View struct {
	Kind         string `json:"kind"`
	SDKVersion   int    `json:"sdkVersion"`
	Source       string `json:"source"`
	StateVersion int    `json:"stateVersion"`
}
type Field struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Unit     string `json:"unit"`
	Nullable bool   `json:"nullable"`
	History  bool   `json:"history"`
}
type Contract struct {
	Format string  `json:"format"`
	Fields []Field `json:"fields"`
}
type Artifact struct {
	Hash   string `json:"hash"`
	View   View   `json:"view"`
	Bundle string `json:"bundle"`
}
type Meta struct {
	SaveError string `json:"saveError,omitempty"`

	Hash      string `json:"hash"`
	Revision  uint64 `json:"revision"`
	BindingID string `json:"bindingId"`
	Sequence  uint64 `json:"sequence"`
}
type Candidate struct {
	Sample          *Data     `json:"sample,omitempty"`
	ProtocolVersion int       `json:"protocolVersion,omitempty"`
	Contract        *Contract `json:"data,omitempty"`

	ID        string `json:"id"`
	PanelID   string `json:"panelId"`
	Title     string `json:"title"`
	Hash      string `json:"hash"`
	ExpiresAt string `json:"expiresAt"`
}
type Point struct {
	Time   string              `json:"time"`
	Values map[string]*float64 `json:"values"`
}
type Data struct {
	BindingID      string         `json:"bindingId"`
	SourceRevision uint64         `json:"sourceRevision"`
	Sequence       uint64         `json:"sequence"`
	Data           map[string]any `json:"data"`
	LastGood       map[string]any `json:"lastGood"`
	UpdatedAt      string         `json:"updatedAt"`
	LastGoodAt     string         `json:"lastGoodAt"`
	Error          string         `json:"error"`
	Issues         []string       `json:"issues"`
	History        []Point        `json:"history"`
	Truncated      bool           `json:"truncated"`
	IntervalMS     int64          `json:"intervalMs"`
}
type Transfer struct {
	Artifacts   []Artifact            `json:"artifacts,omitempty"`
	Data        map[string]Data       `json:"data,omitempty"`
	Definitions map[string]Definition `json:"definitions,omitempty"`
}
type Definition struct {
	Pipeline    *monitorplan.Definition `json:"pipeline,omitempty"`
	ID          string                  `json:"id"`
	Title       string                  `json:"title"`
	Command     string                  `json:"command"`
	IntervalMS  int64                   `json:"intervalMs"`
	Contract    Contract                `json:"data"`
	View        View                    `json:"view"`
	Revision    uint64                  `json:"revision"`
	AllowAIRead bool                    `json:"allowAIRead"`
}

var fieldName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)

func (c Contract) Validate() error {
	if c.Format != "json" && c.Format != "kv" && c.Format != "text" {
		return errors.New("组件数据格式必须为 json、kv 或 text")
	}
	if len(c.Fields) == 0 || len(c.Fields) > 50 {
		return errors.New("数据契约需要 1–50 个字段")
	}
	names := map[string]bool{}
	history := 0
	for _, f := range c.Fields {
		if !fieldName.MatchString(f.Name) || f.Name == "__proto__" || f.Name == "constructor" || f.Name == "prototype" || names[f.Name] {
			return fmt.Errorf("无效或重复字段 %q", f.Name)
		}
		names[f.Name] = true
		switch f.Type {
		case "number", "string", "boolean", "table":
		default:
			return fmt.Errorf("不支持字段类型 %q", f.Type)
		}
		if len(f.Unit) > 32 {
			return errors.New("单位过长")
		}
		if f.History {
			if f.Type != "number" {
				return errors.New("只有数值字段支持历史")
			}
			history++
		}
	}
	if history > 20 {
		return errors.New("最多 20 个历史指标")
	}
	if c.Format == "text" && (len(c.Fields) != 1 || c.Fields[0].Type != "string") {
		return errors.New("text 数据需要单个 string 字段")
	}
	return nil
}
func (v View) Validate() error {
	if v.Kind == "text" {
		return nil
	}
	if v.Kind != "custom" || (v.SDKVersion != SDKVersion && v.SDKVersion != 2) {
		return errors.New("不支持的组件 SDK，请使用 custom / sdkVersion=1 或 2")
	}
	if strings.TrimSpace(v.Source) == "" || len(v.Source) > MaxSource {
		return errors.New("组件源码不能为空且不能超过 128 KiB")
	}
	if v.StateVersion < 1 || v.StateVersion > 1000000 {
		return errors.New("stateVersion 必须为正整数")
	}
	return nil
}
func Hash(v View, bundle string) string {
	b, _ := json.Marshal(struct {
		View             View
		Bundle, Compiler string
	}{v, bundle, "esbuild-0.25.10-sdk-1"})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func (a Artifact) Validate() error {
	if err := a.View.Validate(); err != nil {
		return err
	}
	if a.View.Kind != "custom" || len(a.Bundle) == 0 || len(a.Bundle) > MaxBundle || a.Hash != Hash(a.View, a.Bundle) {
		return errors.New("组件制品校验失败")
	}
	return nil
}

func (d Definition) ValidateData() error {
	if d.Pipeline != nil {
		if d.View.SDKVersion != 2 {
			return errors.New("管线组件需要 SDK v2")
		}
		return d.Pipeline.Validate()
	}
	if d.View.SDKVersion == 2 {
		return errors.New("SDK v2 需要数据管线")
	}
	return d.Contract.Validate()
}
