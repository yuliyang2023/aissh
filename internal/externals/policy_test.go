package externals

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/catalog"
)

func policyFixture() (State, Definition, map[string]any, Caller) {
	op := Definition{ID: "op", Revision: 3, Name: "更新对象", Kind: "http", Lifecycle: "active", InputSchema: EmptyInput(), Assessment: Assessment{Effects: Effects{Access: "write"}, Risk: "medium", Assurance: "reviewed"}}
	g := Grant{ID: "grant", Name: "团队授权", Subject: Subject{Kind: "host", ID: "host-a"}, Operations: map[string]uint64{"op": 3}, Permissions: []string{"write"}, Constraints: map[string]Constraint{"tenant": {Values: []any{"a"}}}}
	return State{Operations: []Definition{op}, Permissions: PermissionPolicy{Enabled: true, Ceiling: Permissions}, Risk: DefaultRisk(), Grants: []Grant{g}}, op, map[string]any{"tenant": "a"}, Caller{HostID: "host-a", SessionID: "session-a"}
}
func TestPermissionAndRiskAreIndependent(t *testing.T) {
	cases := []struct {
		name               string
		change             func(*State, *Definition, map[string]any, *Caller)
		permission, action string
	}{
		{"write needs exact confirmation", func(*State, *Definition, map[string]any, *Caller) {}, "allow", "ask"},
		{"wrong host", func(s *State, d *Definition, a map[string]any, c *Caller) { c.HostID = "host-b" }, "deny", "deny"},
		{"wrong scope", func(s *State, d *Definition, a map[string]any, c *Caller) { a["tenant"] = "b" }, "deny", "deny"},
		{"stale revision", func(s *State, d *Definition, a map[string]any, c *Caller) { d.Revision++ }, "deny", "deny"},
		{"explicit deny wins", func(s *State, d *Definition, a map[string]any, c *Caller) {
			g := s.Grants[0]
			g.ID = "deny"
			g.Deny = true
			s.Grants = append(s.Grants, g)
		}, "deny", "deny"},
		{"expired", func(s *State, d *Definition, a map[string]any, c *Caller) {
			s.Grants[0].ExpiresAt = time.Now().Add(-time.Second).Format(time.RFC3339)
		}, "deny", "deny"},
		{"revoked", func(s *State, d *Definition, a map[string]any, c *Caller) { s.Grants[0].Revoked = true }, "deny", "deny"},
		{"risk cannot grant", func(s *State, d *Definition, a map[string]any, c *Caller) { s.Grants = nil; d.Assessment.Risk = "low" }, "deny", "deny"},
		{"permission cannot defeat risk", func(s *State, d *Definition, a map[string]any, c *Caller) { d.Assessment.Effects.Sensitive = true }, "allow", "deny"},
		{"delete also needs write", func(s *State, d *Definition, a map[string]any, c *Caller) {
			d.Assessment.Effects.Destructive = true
			s.Grants[0].Permissions = []string{"delete"}
		}, "deny", "deny"},
		{"partial grants do not combine", func(s *State, d *Definition, a map[string]any, c *Caller) {
			d.Assessment.Effects.Destructive = true
			g := s.Grants[0]
			g.ID = "second"
			g.Permissions = []string{"delete"}
			s.Grants = append(s.Grants, g)
		}, "deny", "deny"},
		{"unknown risk", func(s *State, d *Definition, a map[string]any, c *Caller) { d.Assessment.Risk = "unknown" }, "deny", "deny"},
		{"inferred cannot run", func(s *State, d *Definition, a map[string]any, c *Caller) { d.Assessment.Assurance = "inferred" }, "deny", "deny"},
		{"single session cannot escape", func(s *State, d *Definition, a map[string]any, c *Caller) {
			s.Grants[0].Subject = Subject{Kind: "session", ID: "session-b"}
		}, "deny", "deny"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			s, d, a, c := policyFixture()
			tt.change(&s, &d, a, &c)
			got := Assess(s, d, a, c, "production", "hash")
			if got.Permission != tt.permission || got.Action != tt.action {
				t.Fatalf("%+v", got)
			}
		})
	}
	s, d, a, c := policyFixture()
	d.Assessment.Effects.Access = "read"
	d.Assessment.Risk = "low"
	s.Grants[0].Permissions = []string{"read"}
	s.Grants[0].AutoRead = true
	if got := Assess(s, d, a, c, "production", "hash"); got.Action != "allow" {
		t.Fatal(got)
	}
	s.Risk.ConfirmRead = true
	if got := Assess(s, d, a, c, "production", "hash"); got.Action != "ask" {
		t.Fatal(got)
	}
}
func TestSchemaAndPreparedRequestCannotExpandScope(t *testing.T) {
	schema := Schema{"type": "object", "properties": map[string]any{"id": Schema{"type": "string", "pattern": "^[a-z0-9]+$"}, "limit": Schema{"type": "integer", "minimum": 1, "maximum": 10}}, "required": []string{"id"}, "additionalProperties": false}
	for _, args := range []map[string]any{{}, {"id": "ok", "url": "https://evil.test"}, {"id": "../root"}, {"id": "ok", "limit": 11}, {"id": "ok", "limit": 1.1}} {
		if err := ValidateArguments(schema, args); err == nil {
			t.Fatalf("accepted %+v", args)
		}
	}
	op := Definition{Kind: "http", InputSchema: schema, Request: Request{Method: "GET", Path: "/assets/{id}", Parameters: []Parameter{{Input: "id", In: "path", Name: "id"}, {Input: "limit", In: "query", Name: "limit"}}}, Assessment: Assessment{Effects: Effects{Access: "read"}}}
	p, err := Prepare(op, map[string]any{"id": "abc", "limit": 3})
	if err != nil || p.Path != "/assets/abc?limit=3" {
		t.Fatal(p, err)
	}
	schema["properties"].(map[string]any)["id"] = Schema{"type": "string"}
	for _, id := range []string{"../x", "%2fadmin", "//evil.test", "x?role=admin"} {
		if _, err := Prepare(op, map[string]any{"id": id}); err == nil {
			t.Fatal(id)
		}
	}
	if err := CheckSchema(Schema{"type": "string", "format": "email"}); err == nil {
		t.Fatal("unsupported format ignored")
	}
	op.Kind = "postgres"
	op.InputSchema = EmptyInput()
	op.Request = Request{Query: "SELECT 1; DELETE FROM assets"}
	if _, err := Prepare(op, map[string]any{}); err == nil {
		t.Fatal("multi SQL accepted")
	}
	op.Kind = "redis"
	op.Request = Request{RedisCommand: "FLUSHDB"}
	op.Assessment.Effects.Access = "write"
	if _, err := Prepare(op, map[string]any{}); err == nil {
		t.Fatal("unbounded Redis command accepted")
	}
}
func TestMigrationCASAndAtomicGrantConsumption(t *testing.T) {
	c := catalog.New(t.TempDir(), nil)
	r, err := c.SaveResource(catalog.Resource{Name: "Redis", Kind: "redis", Endpoint: "localhost:6379", AllowLocal: true, Grant: "read"})
	if err != nil {
		t.Fatal(err)
	}
	s := NewStore(c)
	st, err := s.Ensure(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Load()
	if d.Version != 2 || !d.Resources[0].Managed || len(st.Grants) != 1 {
		t.Fatal(d, st)
	}
	if _, err := os.Stat(filepath.Join(c.Dir, "connections.v1.backup.json")); err != nil {
		t.Fatal("no migration backup", err)
	}
	st.Grants[0].MaxUses = 1
	st, err = s.SavePermissions(r.ID, st.Revision, st.Permissions, st.Grants, "限单次读取")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveRisk(r.ID, st.Revision-1, st.Risk, "stale"); err == nil {
		t.Fatal("stale update accepted")
	}
	var success atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.Consume(r.ID, st.Revision, st.Grants[0].ID, Invocation{ID: catalog.NewID()}) == nil {
				success.Add(1)
			}
		}()
	}
	wg.Wait()
	if success.Load() != 1 {
		t.Fatal("one-use grant raced", success.Load())
	}
	st, _ = s.Read(r.ID)
	if st.Grants[0].Used != 1 {
		t.Fatal(st.Grants)
	}
	if err := os.Remove(filepath.Join(s.dir(r.ID), "catalog.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Ensure(r.ID); err == nil {
		t.Fatal("missing catalog reset permissions")
	}
}
