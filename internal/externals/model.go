// Package externals owns versioned external operations, permissions and risk.
// Connections and secrets remain in catalog; models never receive credentials.
package externals

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

type Schema map[string]any

type Parameter struct {
	Input string `json:"input"`
	In    string `json:"in"` // path, query, body
	Name  string `json:"name"`
}

type Request struct {
	Method          string      `json:"method,omitempty"`
	Path            string      `json:"path,omitempty"`
	Parameters      []Parameter `json:"parameters"`
	BodyInput       string      `json:"bodyInput,omitempty"`
	Query           string      `json:"query,omitempty"`
	SQLParameters   []string    `json:"sqlParameters,omitempty"`
	RedisCommand    string      `json:"redisCommand,omitempty"`
	RedisParameters []string    `json:"redisParameters,omitempty"`
}

type Effects struct {
	Access           string `json:"access"` // read, write, unknown
	Destructive      bool   `json:"destructive"`
	ServiceImpact    bool   `json:"serviceImpact"`
	PermissionChange bool   `json:"permissionChange"`
	Sensitive        bool   `json:"sensitive"`
}

type Assessment struct {
	Effects    Effects `json:"effects"`
	Risk       string  `json:"risk"`      // low, medium, high, critical, unknown
	Assurance  string  `json:"assurance"` // inferred, reviewed, builtin
	Evidence   string  `json:"evidence"`
	ReviewedAt string  `json:"reviewedAt,omitempty"`
}

type Definition struct {
	ID               string     `json:"id"`
	Revision         uint64     `json:"revision"`
	Name             string     `json:"name"`
	Kind             string     `json:"kind"`
	Description      string     `json:"description"`
	InputSchema      Schema     `json:"inputSchema"`
	OutputSchema     Schema     `json:"outputSchema,omitempty"`
	Request          Request    `json:"request"`
	Assessment       Assessment `json:"assessment"`
	Lifecycle        string     `json:"lifecycle"` // draft, active, disabled, deprecated
	Source           string     `json:"source"`
	Blocked          string     `json:"blocked,omitempty"`
	QuantityArgument string     `json:"quantityArgument,omitempty"`
	ChangeReason     string     `json:"changeReason,omitempty"`
	UpdatedAt        string     `json:"updatedAt"`
}

type Subject struct {
	Kind string `json:"kind"` // local, host, session
	ID   string `json:"id,omitempty"`
}

type Constraint struct {
	Values  []any    `json:"values,omitempty"`
	Prefix  string   `json:"prefix,omitempty"`
	Maximum *float64 `json:"maximum,omitempty"`
}

type Grant struct {
	ID            string                `json:"id"`
	Name          string                `json:"name"`
	Subject       Subject               `json:"subject"`
	Operations    map[string]uint64     `json:"operations"` // exact revisions
	Permissions   []string              `json:"permissions"`
	Constraints   map[string]Constraint `json:"constraints"`
	AllowedFields []string              `json:"allowedFields"` // input leaf paths; empty means all schema fields
	ValidFrom     string                `json:"validFrom,omitempty"`
	ExpiresAt     string                `json:"expiresAt,omitempty"`
	Revoked       bool                  `json:"revoked"`
	Deny          bool                  `json:"deny"`
	AutoRead      bool                  `json:"autoRead"`
	MaxUses       int                   `json:"maxUses"` // 0 means not counted
	Used          int                   `json:"used"`
	RequestDigest string                `json:"requestDigest,omitempty"`
	GrantedBy     string                `json:"grantedBy"`
	UpdatedAt     string                `json:"updatedAt"`
}

type PermissionPolicy struct {
	Enabled          bool     `json:"enabled"`
	Ceiling          []string `json:"ceiling"`
	DeniedOperations []string `json:"deniedOperations"`
}

type RiskPolicy struct {
	DenySensitive    bool   `json:"denySensitive"`
	DenyDestructive  bool   `json:"denyDestructive"`
	AllowHigh        bool   `json:"allowHigh"`
	DenyCritical     bool   `json:"denyCritical"`
	ConfirmRead      bool   `json:"confirmRead"`
	ProductionFloor  string `json:"productionFloor"`
	MaxQuantity      int    `json:"maxQuantity"`
	MaxResponseBytes int    `json:"maxResponseBytes"`
	MaxRows          int    `json:"maxRows"`
	TimeoutSeconds   int    `json:"timeoutSeconds"`
}

type Change struct {
	Time     string `json:"time"`
	Actor    string `json:"actor"`
	Kind     string `json:"kind"`
	Reason   string `json:"reason"`
	Revision uint64 `json:"revision"`
}

type State struct {
	Version           int              `json:"version"`
	ConnectionID      string           `json:"connectionId"`
	Revision          uint64           `json:"revision"`
	DefinitionBinding string           `json:"definitionBinding"`
	Operations        []Definition     `json:"operations"`
	Candidates        []Definition     `json:"candidates"`
	History           []Definition     `json:"history"`
	Grants            []Grant          `json:"grants"`
	Permissions       PermissionPolicy `json:"permissions"`
	Risk              RiskPolicy       `json:"risk"`
	Changes           []Change         `json:"changes"`
}

type Caller struct {
	HostID    string `json:"hostId"`
	SessionID string `json:"sessionId"`
	Local     bool   `json:"local"`
}

type Decision struct {
	Permission string   `json:"permission"`
	Risk       string   `json:"risk"`
	Action     string   `json:"action"`
	Reasons    []string `json:"reasons"`
	Required   []string `json:"required"`
	GrantID    string   `json:"grantId,omitempty"`
}

type Invocation struct {
	ID                string   `json:"id"`
	Time              string   `json:"time"`
	ConnectionID      string   `json:"connectionId"`
	OperationID       string   `json:"operationId"`
	OperationRevision uint64   `json:"operationRevision"`
	Caller            string   `json:"caller"`
	StateRevision     uint64   `json:"stateRevision"`
	RequestDigest     string   `json:"requestDigest"`
	Decision          Decision `json:"decision"`
	Approved          bool     `json:"approved"`
	Outcome           string   `json:"outcome"`
	Summary           string   `json:"summary"`
}

var Permissions = []string{"read", "write", "delete", "operate", "remote_admin"}

func Now() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func Digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func OperationID(kind, key string) string { return "op-" + Digest([]string{kind, key})[:24] }
func DefaultRisk() RiskPolicy {
	return RiskPolicy{DenySensitive: true, DenyDestructive: true, DenyCritical: true, ProductionFloor: "medium", MaxQuantity: 100, MaxResponseBytes: 128 * 1024, MaxRows: 100, TimeoutSeconds: 30}
}
func RiskRank(r string) int {
	switch r {
	case "low":
		return 0
	case "medium":
		return 1
	case "high":
		return 2
	case "critical":
		return 3
	default:
		return 4
	}
}
func Required(e Effects) []string {
	p := []string{}
	switch e.Access {
	case "read":
		p = append(p, "read")
	case "write":
		p = append(p, "write")
	default:
		return []string{"unknown"}
	}
	if e.Destructive {
		if !slices.Contains(p, "write") {
			p = append(p, "write")
		}
		p = append(p, "delete")
	}
	if e.ServiceImpact {
		p = append(p, "operate")
	}
	if e.PermissionChange {
		p = append(p, "remote_admin")
	}
	return p
}
func ValidateDefinition(d Definition) error {
	if d.ID == "" || len(d.ID) > 100 || strings.TrimSpace(d.Name) == "" || len(d.Name) > 512 {
		return errors.New("接口 ID 或名称无效")
	}
	if !slices.Contains([]string{"http", "postgres", "mysql", "redis"}, d.Kind) {
		return errors.New("不支持的接口类型")
	}
	if !slices.Contains([]string{"draft", "active", "disabled", "deprecated"}, d.Lifecycle) {
		return errors.New("接口状态无效")
	}
	if !slices.Contains([]string{"read", "write", "unknown"}, d.Assessment.Effects.Access) || !slices.Contains([]string{"low", "medium", "high", "critical", "unknown"}, d.Assessment.Risk) {
		return errors.New("接口效果或风险无效")
	}
	if err := CheckSchema(d.InputSchema); err != nil {
		return fmt.Errorf("输入定义：%w", err)
	}
	if len(d.OutputSchema) > 0 {
		if d.Kind != "http" {
			return errors.New("输出 Schema 校验目前仅支持 HTTP JSON 响应")
		}
		if err := CheckSchema(d.OutputSchema); err != nil {
			return fmt.Errorf("输出定义：%w", err)
		}
	}
	if d.InputSchema["type"] != "object" {
		return errors.New("接口输入必须为 object")
	}
	return nil
}
