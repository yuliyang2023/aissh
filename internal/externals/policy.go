package externals

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
)

func SubjectMatches(s Subject, c Caller) bool {
	switch s.Kind {
	case "local":
		return c.Local
	case "host":
		return !c.Local && s.ID != "" && s.ID == c.HostID
	case "session":
		return s.ID != "" && s.ID == c.SessionID
	}
	return false
}
func ActiveGrant(g Grant, now time.Time) bool {
	if g.Revoked || g.MaxUses > 0 && g.Used >= g.MaxUses {
		return false
	}
	if g.ValidFrom != "" {
		t, e := time.Parse(time.RFC3339Nano, g.ValidFrom)
		if e != nil || now.Before(t) {
			return false
		}
	}
	if g.ExpiresAt != "" {
		t, e := time.Parse(time.RFC3339Nano, g.ExpiresAt)
		if e != nil || !now.Before(t) {
			return false
		}
	}
	return true
}
func HasScope(s State, c Caller) bool {
	for _, g := range s.Grants {
		if !g.Deny && ActiveGrant(g, time.Now()) && SubjectMatches(g.Subject, c) {
			return true
		}
	}
	return false
}
func Assess(s State, d Definition, args map[string]any, c Caller, environment, requestDigest string) Decision {
	dec := Decision{Permission: "deny", Risk: d.Assessment.Risk, Action: "deny", Reasons: []string{}, Required: Required(d.Assessment.Effects)}
	deny := func(reason string) Decision { dec.Reasons = append(dec.Reasons, reason); return dec }
	if !s.Permissions.Enabled {
		return deny("此连接已暂停")
	}
	if d.Lifecycle != "active" || d.Blocked != "" {
		return deny("接口尚未启用或定义不完整")
	}
	if d.Assessment.Assurance != "reviewed" && d.Assessment.Assurance != "builtin" {
		return deny("接口效果仅为推断，请先在操作目录确认定义与效果")
	}
	if d.Assessment.Risk == "unknown" {
		return deny("接口风险尚未确认")
	}
	if slices.Contains(dec.Required, "unknown") {
		return deny("无法确定接口所需能力，暂不执行")
	}
	if slices.Contains(s.Permissions.DeniedOperations, d.ID) {
		return deny("接口命中连接的显式禁止规则")
	}
	for _, p := range dec.Required {
		if !slices.Contains(s.Permissions.Ceiling, p) {
			return deny("连接权限上限未允许 " + p)
		}
	}
	candidates := []Grant{}
	for _, g := range s.Grants {
		if !SubjectMatches(g.Subject, c) || !ActiveGrant(g, time.Now()) {
			continue
		}
		rev, ok := g.Operations[d.ID]
		if !ok || rev != d.Revision {
			continue
		}
		if g.RequestDigest != "" && g.RequestDigest != requestDigest {
			continue
		}
		if !constraintsMatch(g, args) {
			continue
		}
		if g.Deny {
			return deny("请求命中显式禁止授权：" + g.Name)
		}
		covers := true
		for _, p := range dec.Required {
			if !slices.Contains(g.Permissions, p) {
				covers = false
			}
		}
		if covers {
			candidates = append(candidates, g)
		}
	}
	if len(candidates) == 0 {
		return deny("当前调用者没有覆盖此接口版本、能力和参数范围的有效授权")
	}
	// Deterministic selection; a bounded grant is consumed only after approval.
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	grant := candidates[0]
	dec.Permission = "allow"
	dec.GrantID = grant.ID
	dec.Reasons = append(dec.Reasons, "权限通过："+grant.Name)
	e := d.Assessment.Effects
	if e.Access == "write" && RiskRank(dec.Risk) < 1 {
		dec.Risk = "medium"
	}
	if e.Destructive || e.ServiceImpact || e.PermissionChange {
		if RiskRank(dec.Risk) < 2 {
			dec.Risk = "high"
		}
	}
	if environment != "test" && environment != "development" && e.Access != "read" && RiskRank(s.Risk.ProductionFloor) > RiskRank(dec.Risk) {
		dec.Risk = s.Risk.ProductionFloor
		dec.Reasons = append(dec.Reasons, "生产或未标明环境的风险下限")
	}
	if e.Sensitive && s.Risk.DenySensitive {
		return deny("风险规则禁止敏感数据操作")
	}
	if e.Destructive && s.Risk.DenyDestructive {
		return deny("风险规则禁止删除或清空数据")
	}
	if dec.Risk == "critical" && s.Risk.DenyCritical {
		return deny("风险规则禁止严重风险操作")
	}
	if (dec.Risk == "high" || dec.Risk == "critical") && !s.Risk.AllowHigh {
		return deny("尚未启用高风险接口执行")
	}
	if d.QuantityArgument != "" {
		v, ok := lookup(args, d.QuantityArgument)
		n, nok := number(v)
		if !ok || !nok || n < 1 || n > float64(s.Risk.MaxQuantity) {
			return deny("数量参数缺失或超过风险规则上限")
		}
	}
	dec.Action = "ask"
	if dec.Risk == "low" && e.Access == "read" && !e.Sensitive && !e.Destructive && !e.ServiceImpact && !e.PermissionChange && grant.AutoRead && !s.Risk.ConfirmRead {
		dec.Action = "allow"
		dec.Reasons = append(dec.Reasons, "已授权的低风险读取可自动执行")
	} else {
		dec.Reasons = append(dec.Reasons, "风险规则要求确认本次精确请求")
	}
	return dec
}
func constraintsMatch(g Grant, args map[string]any) bool {
	for name, c := range g.Constraints {
		v, ok := lookup(args, name)
		if !ok {
			return false
		}
		if len(c.Values) > 0 {
			match := false
			for _, x := range c.Values {
				if equal(v, x) {
					match = true
				}
			}
			if !match {
				return false
			}
		}
		if c.Prefix != "" {
			s, ok := v.(string)
			if !ok || !strings.HasPrefix(s, c.Prefix) {
				return false
			}
		}
		if c.Maximum != nil {
			n, ok := number(v)
			if !ok || n > *c.Maximum {
				return false
			}
		}
	}
	if len(g.AllowedFields) > 0 {
		for _, name := range leaves(args, "") {
			if !slices.Contains(g.AllowedFields, name) {
				return false
			}
		}
	}
	return true
}
func lookup(args map[string]any, path string) (any, bool) {
	var v any = args
	for _, p := range strings.Split(path, ".") {
		m, ok := object(v)
		if !ok {
			return nil, false
		}
		v, ok = m[p]
		if !ok {
			return nil, false
		}
	}
	return v, true
}
func leaves(v any, prefix string) []string {
	m, ok := object(v)
	if !ok || len(m) == 0 {
		return []string{prefix}
	}
	out := []string{}
	for k, v := range m {
		name := k
		if prefix != "" {
			name = prefix + "." + k
		}
		out = append(out, leaves(v, name)...)
	}
	return out
}
func ValidatePolicies(p PermissionPolicy, r RiskPolicy, grants []Grant) error {
	for _, v := range p.Ceiling {
		if !slices.Contains(Permissions, v) {
			return fmt.Errorf("无效能力 %s", v)
		}
	}
	if !slices.Contains([]string{"low", "medium", "high", "critical"}, r.ProductionFloor) || r.MaxQuantity < 1 || r.MaxQuantity > 10000 || r.MaxRows < 1 || r.MaxRows > 100 || r.MaxResponseBytes < 1024 || r.MaxResponseBytes > 1024*1024 || r.TimeoutSeconds < 1 || r.TimeoutSeconds > 60 {
		return fmt.Errorf("风险阈值无效：数量 1–10000，结果 1–100 行/1KB–1MB，超时 1–60 秒")
	}
	if len(grants) > 200 {
		return fmt.Errorf("每个连接最多 200 项授权")
	}
	seen := map[string]bool{}
	for _, g := range grants {
		if g.ID != "" && seen[g.ID] {
			return fmt.Errorf("授权 ID 重复")
		}
		seen[g.ID] = true
		if strings.TrimSpace(g.Name) == "" || len(g.Name) > 200 || !slices.Contains([]string{"local", "host", "session"}, g.Subject.Kind) || g.Subject.Kind != "local" && g.Subject.ID == "" || g.MaxUses < 0 {
			return fmt.Errorf("授权名称、主体或使用次数无效")
		}
		for _, p := range g.Permissions {
			if !slices.Contains(Permissions, p) {
				return fmt.Errorf("无效权限 %s", p)
			}
		}
		for _, date := range []string{g.ValidFrom, g.ExpiresAt} {
			if date != "" {
				if _, e := time.Parse(time.RFC3339Nano, date); e != nil {
					return fmt.Errorf("授权有效期必须是 RFC3339 时间")
				}
			}
		}
		if g.ValidFrom != "" && g.ExpiresAt != "" {
			start, _ := time.Parse(time.RFC3339Nano, g.ValidFrom)
			end, _ := time.Parse(time.RFC3339Nano, g.ExpiresAt)
			if !end.After(start) {
				return fmt.Errorf("授权结束时间必须晚于开始时间")
			}
		}
		if len(g.Operations) == 0 && !g.Revoked {
			return fmt.Errorf("授权需要明确选择接口版本")
		}
		for name, c := range g.Constraints {
			if name == "" || len(c.Values) == 0 && c.Prefix == "" && c.Maximum == nil {
				return fmt.Errorf("参数约束需要指定允许值、前缀或上限")
			}
		}
	}
	return nil
}
