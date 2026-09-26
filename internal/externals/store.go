package externals

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"git.example.com/ai/wrd-ai-cli/internal/catalog"
	"github.com/gofrs/flock"
)

type Store struct{ Catalog *catalog.Store }

func NewStore(c *catalog.Store) *Store { return &Store{Catalog: c} }
func (s *Store) dir(id string) string  { return filepath.Join(s.Catalog.Dir, "external-systems", id) }
func (s *Store) lock(id string) (*flock.Flock, error) {
	if catalog.ValidateID(id) != nil {
		return nil, errors.New("连接标识无效")
	}
	if err := os.MkdirAll(s.dir(id), 0700); err != nil {
		return nil, err
	}
	l := flock.New(filepath.Join(s.dir(id), "catalog.lock"))
	return l, l.Lock()
}
func (s *Store) Read(id string) (State, error) {
	var st State
	if catalog.ValidateID(id) != nil {
		return st, errors.New("连接标识无效")
	}
	f, err := os.Open(filepath.Join(s.dir(id), "catalog.json"))
	if err != nil {
		return st, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 8*1024*1024+1))
	if err != nil {
		return st, err
	}
	if len(b) > 8*1024*1024 || json.Unmarshal(b, &st) != nil || st.Version != 1 || st.ConnectionID != id {
		return State{}, errors.New("外联接口目录损坏或版本不受支持；已拒绝调用")
	}
	if err := ValidatePolicies(st.Permissions, st.Risk, st.Grants); err != nil {
		return State{}, fmt.Errorf("外联策略损坏，拒绝调用：%w", err)
	}
	return st, nil
}
func atomicJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if len(b) > 8*1024*1024 {
		return errors.New("外联目录超过 8 MB")
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".external-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err = f.Chmod(0600); err != nil {
		return err
	}
	if _, err = f.Write(b); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
func (s *Store) write(st State) error {
	return atomicJSON(filepath.Join(s.dir(st.ConnectionID), "catalog.json"), st)
}
func DefinitionBinding(r catalog.Resource, d catalog.Data) string {
	route := []any{}
	if r.Route == "ssh" {
		id := r.HostID
		for i := 0; id != "" && i < 8; i++ {
			h, err := d.Host(id)
			if err != nil {
				route = append(route, "missing:"+id)
				break
			}
			route = append(route, []any{h.ID, h.Address, h.Port, h.User, h.JumpHostID})
			id = h.JumpHostID
		}
	}
	return Digest([]any{r.Kind, r.Endpoint, r.Database, r.Route, r.TLS, route})
}
func (s *Store) Ensure(id string) (State, error) {
	d, err := s.Catalog.Load()
	if err != nil {
		return State{}, err
	}
	r, err := d.Resource(id)
	if err != nil {
		return State{}, err
	}
	l, err := s.lock(id)
	if err != nil {
		return State{}, err
	}
	st, err := s.Read(id)
	if errors.Is(err, os.ErrNotExist) {
		if r.Managed {
			l.Unlock()
			return State{}, errors.New("已管理连接的接口目录丢失，请从备份恢复；未恢复默认权限")
		}
		st = State{Version: 1, ConnectionID: id, Revision: 1, DefinitionBinding: DefinitionBinding(r, d), Operations: Builtins(r.Kind), Candidates: []Definition{}, History: []Definition{}, Grants: []Grant{}, Permissions: PermissionPolicy{Enabled: true, Ceiling: []string{"read"}, DeniedOperations: []string{}}, Risk: DefaultRisk(), Changes: []Change{{Time: Now(), Actor: "migration", Kind: "migration", Reason: "迁移原只读范围；发现的接口仍待确认", Revision: 1}}}
		refs := map[string]uint64{}
		for _, op := range st.Operations {
			refs[op.ID] = op.Revision
		}
		subjects := []Subject{}
		if r.HostID != "" {
			subjects = append(subjects, Subject{Kind: "host", ID: r.HostID})
		}
		if r.AllowLocal {
			subjects = append(subjects, Subject{Kind: "local"})
		}
		if len(refs) > 0 {
			for _, subject := range subjects {
				st.Grants = append(st.Grants, Grant{ID: catalog.NewID(), Name: "迁移的只读授权", Subject: subject, Operations: refs, Permissions: []string{"read"}, Constraints: map[string]Constraint{}, AllowedFields: []string{}, AutoRead: r.Grant == "read", GrantedBy: "migration", UpdatedAt: Now()})
			}
		}
		if r.Discovery != nil {
			st.Candidates = LegacyCandidates(*r.Discovery, r.Kind)
		}
		err = s.write(st)
		if err == nil && r.Discovery != nil {
			err = atomicJSON(filepath.Join(s.dir(id), "discovery.json"), r.Discovery)
		}
	}
	l.Unlock()
	if err != nil {
		return st, err
	}
	// The prepared catalog is activated only after the version gate is committed.
	if !r.Managed {
		if err := s.Catalog.EnableExternalPolicies(id); err != nil {
			return st, err
		}
	}
	return st, nil
}
func (s *Store) mutate(id string, revision uint64, kind, reason string, fn func(*State) error) (State, error) {
	if strings.TrimSpace(reason) == "" || len(reason) > 1024 {
		return State{}, errors.New("请填写不超过 1024 字节的变更原因")
	}
	if _, err := s.Ensure(id); err != nil {
		return State{}, err
	}
	l, err := s.lock(id)
	if err != nil {
		return State{}, err
	}
	defer l.Unlock()
	st, err := s.Read(id)
	if err != nil {
		return st, err
	}
	if st.Revision != revision {
		return st, errors.New("目录或规则已更新，请刷新后重试")
	}
	if err := fn(&st); err != nil {
		return st, err
	}
	st.Revision++
	st.Changes = append(st.Changes, Change{Time: Now(), Actor: "local-owner", Kind: kind, Reason: reason, Revision: st.Revision})
	if len(st.Changes) > 200 {
		st.Changes = st.Changes[len(st.Changes)-200:]
	}
	return st, s.write(st)
}
func (s *Store) SaveOperation(id string, revision uint64, op Definition, reason string) (State, error) {
	return s.mutate(id, revision, "operation", reason, func(st *State) error {
		d, err := s.Catalog.Load()
		if err != nil {
			return err
		}
		r, err := d.Resource(id)
		if err != nil {
			return err
		}
		if op.Kind != r.Kind {
			return errors.New("接口类型与连接不一致")
		}
		if op.ID == "" {
			op.ID = "op-" + catalog.NewID()
		}
		if op.Lifecycle == "active" && (op.Blocked != "" || op.Assessment.Effects.Access == "unknown" || op.Assessment.Risk == "unknown") {
			return errors.New("启用前请确认效果、风险并解决定义问题")
		}
		if err := ValidateDefinition(op); err != nil {
			return err
		}
		i := slices.IndexFunc(st.Operations, func(v Definition) bool { return v.ID == op.ID })
		op.Revision = 1
		if i >= 0 {
			old := st.Operations[i]
			op.Revision = old.Revision + 1
			st.History = append(st.History, old)
		}
		op.Assessment.Assurance = "reviewed"
		op.Assessment.ReviewedAt = Now()
		op.UpdatedAt = Now()
		op.ChangeReason = reason
		if i >= 0 {
			st.Operations[i] = op
		} else {
			st.Operations = append(st.Operations, op)
		}
		st.Candidates = slices.DeleteFunc(st.Candidates, func(v Definition) bool { return v.ID == op.ID })
		if len(st.History) > 200 {
			st.History = st.History[len(st.History)-200:]
		}
		if st.DefinitionBinding != DefinitionBinding(r, d) {
			for j := range st.Operations {
				if st.Operations[j].ID != op.ID {
					st.Operations[j].Lifecycle = "draft"
				}
			}
		}
		st.DefinitionBinding = DefinitionBinding(r, d)
		return nil
	})
}
func (s *Store) SavePermissions(id string, revision uint64, p PermissionPolicy, grants []Grant, reason string) (State, error) {
	return s.mutate(id, revision, "permissions", reason, func(st *State) error {
		if err := ValidatePolicies(p, st.Risk, grants); err != nil {
			return err
		}
		d, err := s.Catalog.Load()
		if err != nil {
			return err
		}
		for i := range grants {
			g := &grants[i]
			if g.Subject.Kind == "host" {
				if _, err := d.Host(g.Subject.ID); err != nil {
					return err
				}
			}
			for opID, rev := range g.Operations {
				found := false
				for _, op := range st.Operations {
					if op.ID == opID && op.Revision == rev {
						found = true
					}
				}
				if !found && !g.Revoked {
					return fmt.Errorf("授权引用了不存在或已变更的接口版本 %s", opID)
				}
			}
			if g.ID == "" {
				g.ID = catalog.NewID()
			}
			g.Used = 0
			for _, old := range st.Grants {
				if old.ID == g.ID {
					g.Used = old.Used
				}
			}
			g.GrantedBy = "local-owner"
			g.UpdatedAt = Now()
		}
		st.Permissions = p
		st.Grants = grants
		return nil
	})
}
func (s *Store) SaveRisk(id string, revision uint64, r RiskPolicy, reason string) (State, error) {
	return s.mutate(id, revision, "risk", reason, func(st *State) error {
		if err := ValidatePolicies(st.Permissions, r, st.Grants); err != nil {
			return err
		}
		st.Risk = r
		return nil
	})
}
func (s *Store) Import(id string, report catalog.ResourceDiscovery, defs []Definition) error {
	if _, err := s.Ensure(id); err != nil {
		return err
	}
	l, err := s.lock(id)
	if err != nil {
		return err
	}
	defer l.Unlock()
	st, err := s.Read(id)
	if err != nil {
		return err
	}
	data, err := s.Catalog.Load()
	if err != nil {
		return err
	}
	r, err := data.Resource(id)
	if err != nil {
		return err
	}
	if catalog.ResourceBinding(r, data) != report.Binding {
		return errors.New("连接已经改变，探索结果只保留为历史")
	}
	seen := map[string]bool{}
	for _, op := range defs {
		seen[op.ID] = true
		index := slices.IndexFunc(st.Operations, func(v Definition) bool { return v.ID == op.ID })
		if index >= 0 {
			old := st.Operations[index]
			if Digest(old.Request) == Digest(op.Request) && Digest(old.InputSchema) == Digest(op.InputSchema) && Digest(old.Assessment.Effects) == Digest(op.Assessment.Effects) && Digest(old.OutputSchema) == Digest(op.OutputSchema) && old.Description == op.Description && old.Blocked == op.Blocked && old.Assessment.Risk == op.Assessment.Risk {
				continue
			}
			if old.Assessment.Assurance == "builtin" {
				continue
			}
			st.Operations[index].Lifecycle = "draft"
		}
		op.Assessment.Assurance = "inferred"
		op.Lifecycle = "draft"
		op.UpdatedAt = Now()
		ci := slices.IndexFunc(st.Candidates, func(v Definition) bool { return v.ID == op.ID })
		if ci >= 0 {
			st.Candidates[ci] = op
		} else {
			st.Candidates = append(st.Candidates, op)
		}
	}
	if report.Status == "ready" {
		for i, op := range st.Operations {
			if op.Source == "discovery" && !seen[op.ID] {
				st.Operations[i].Lifecycle = "deprecated"
			}
		}
	}
	if len(st.Candidates) > 500 {
		st.Candidates = st.Candidates[:500]
	}
	// Keep definitions on a failed/partial scan and record its coverage separately.
	if err := atomicJSON(filepath.Join(s.dir(id), "discovery.json"), report); err != nil {
		return err
	}
	st.Revision++
	return s.write(st)
}
func (s *Store) Discovery(id string) (*catalog.ResourceDiscovery, error) {
	f, err := os.Open(filepath.Join(s.dir(id), "discovery.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var report catalog.ResourceDiscovery
	err = json.NewDecoder(io.LimitReader(f, 1024*1024)).Decode(&report)
	return &report, err
}

// Reusing a system's contracts does not copy credentials, grants or assurance.
func (s *Store) CopyDefinitions(source, target string, revision uint64, reason string) (State, error) {
	original, err := s.Ensure(source)
	if err != nil {
		return State{}, err
	}
	data, err := s.Catalog.Load()
	if err != nil {
		return State{}, err
	}
	src, err := data.Resource(source)
	if err != nil {
		return State{}, err
	}
	dst, err := data.Resource(target)
	if err != nil {
		return State{}, err
	}
	if source == target || src.Kind != dst.Kind || src.SystemID == "" || src.SystemID != dst.SystemID {
		return State{}, errors.New("只能复用同一系统且类型相同的另一连接")
	}
	return s.mutate(target, revision, "copy-definitions", reason, func(st *State) error {
		for _, op := range original.Operations {
			if op.Assessment.Assurance == "builtin" {
				continue
			}
			op.Revision = 1
			op.Assessment.Assurance = "inferred"
			op.Assessment.ReviewedAt = ""
			op.Lifecycle = "draft"
			op.Source = "copied:" + source
			op.UpdatedAt = Now()
			i := slices.IndexFunc(st.Candidates, func(v Definition) bool { return v.ID == op.ID })
			if i >= 0 {
				st.Candidates[i] = op
			} else {
				st.Candidates = append(st.Candidates, op)
			}
		}
		if len(st.Candidates) > 500 {
			return errors.New("候选接口不能超过 500 项")
		}
		return nil
	})
}
