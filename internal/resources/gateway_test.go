package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"git.example.com/ai/wrd-ai-cli/internal/catalog"
	"git.example.com/ai/wrd-ai-cli/internal/externals"
)

func registerHTTP(t *testing.T, e *Executor, r catalog.Resource, method, path, access string, auto bool) (Operation, externals.State) {
	t.Helper()
	store := externals.NewStore(e.Store)
	st, err := store.Ensure(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	risk := "low"
	if access == "write" {
		risk = "medium"
	}
	def := externals.Definition{ID: externals.OperationID("http", method+path), Name: method + path, Kind: "http", Lifecycle: "active", InputSchema: externals.EmptyInput(), Request: externals.Request{Method: method, Path: path, Parameters: []externals.Parameter{}}, Assessment: externals.Assessment{Effects: externals.Effects{Access: access}, Risk: risk}, Source: "manual"}
	st, err = store.SaveOperation(r.ID, st.Revision, def, "测试接口审查")
	if err != nil {
		t.Fatal(err)
	}
	def = st.Operations[len(st.Operations)-1]
	st.Permissions.Ceiling = []string{"read", "write"}
	st.Grants = append(st.Grants, externals.Grant{ID: catalog.NewID(), Name: "精确接口授权", Subject: externals.Subject{Kind: "local"}, Operations: map[string]uint64{def.ID: def.Revision}, Permissions: []string{access}, AutoRead: auto, Constraints: map[string]externals.Constraint{}, AllowedFields: []string{}})
	st, err = store.SavePermissions(r.ID, st.Revision, st.Permissions, st.Grants, "测试授权")
	if err != nil {
		t.Fatal(err)
	}
	return Operation{ResourceID: r.ID, Action: "system.invoke", OperationID: def.ID, OperationRevision: def.Revision, Arguments: map[string]any{}}, st
}
func TestGatewayWriteNeedsExactCurrentApproval(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Method != "POST" || r.URL.Path != "/api/items" || r.Header.Get("Authorization") != "Bearer private-test-token" {
			t.Errorf("wrong request: %s %s", r.Method, r.URL)
		}
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer server.Close()
	e, r := setup(t, server.URL+"/api")
	op, st := registerHTTP(t, e, r, "POST", "/items", "write", true)
	if _, err := e.Execute(context.Background(), op, true); err == nil {
		t.Fatal("approval without preview")
	}
	if _, err := e.Execute(context.Background(), op, false); err == nil {
		t.Fatal("write auto-ran")
	}
	preview, err := e.Preview(op)
	if err != nil || preview.Decision.Action != "ask" {
		t.Fatal(preview, err)
	}
	st.Risk.ConfirmRead = true
	st, err = externals.NewStore(e.Store).SaveRisk(r.ID, st.Revision, st.Risk, "规则发生变化")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.ExecutePreview(context.Background(), preview.Token); err == nil {
		t.Fatal("stale approval survived")
	}
	if hits.Load() != 0 {
		t.Fatal("blocked request sent")
	}
	preview, err = e.Preview(op)
	if err != nil {
		t.Fatal(err)
	}
	out, err := e.ExecutePreview(context.Background(), preview.Token)
	if err != nil || !strings.Contains(out, "succeeded") || hits.Load() != 1 {
		t.Fatal(out, err, hits.Load())
	}
	if _, err := e.ExecutePreview(context.Background(), preview.Token); err == nil || hits.Load() != 1 {
		t.Fatal("approval replayed", err)
	}
	inv, err := externals.NewStore(e.Store).Invocations(r.ID)
	if err != nil || len(inv) != 2 || inv[0].Outcome != "succeeded" || inv[1].Outcome != "started" {
		t.Fatal(inv, err)
	}
	for _, legacy := range []Operation{{ResourceID: r.ID, Action: "http.get", Path: "/items"}, {ResourceID: r.ID, Action: "system.invoke", OperationID: op.OperationID, OperationRevision: op.OperationRevision, Path: "/other"}} {
		if _, err := e.Execute(context.Background(), legacy, true); err == nil {
			t.Fatal("raw escape accepted")
		}
	}
}
func TestGatewayOneUseAcrossExecutorsAndRevocation(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); fmt.Fprint(w, "ok") }))
	defer server.Close()
	e, r := setup(t, server.URL)
	op, st := registerHTTP(t, e, r, "GET", "/health", "read", true)
	st.Grants[0].MaxUses = 1
	store := externals.NewStore(e.Store)
	st, err := store.SavePermissions(r.ID, st.Revision, st.Permissions, st.Grants, "单次授权")
	if err != nil {
		t.Fatal(err)
	}
	e2 := New(e.Store, "", "", nil)
	if _, err := e.Execute(context.Background(), op, false); err != nil {
		t.Fatal(err)
	}
	if _, err := e2.Execute(context.Background(), op, false); err == nil || hits.Load() != 1 {
		t.Fatal("grant limit bypassed", err)
	}
	st, _ = store.Read(r.ID)
	st.Grants[0].MaxUses = 0
	st.Grants[0].Revoked = true
	_, err = store.SavePermissions(r.ID, st.Revision, st.Permissions, st.Grants, "撤销")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Execute(context.Background(), op, true); err == nil {
		t.Fatal("approval defeated revoke")
	}
}
func TestGatewaySchemaPrecedesNetworkAndUnknownWriteNotRetried(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		conn, _, _ := w.(http.Hijacker).Hijack()
		conn.Close()
	}))
	defer server.Close()
	e, r := setup(t, server.URL)
	op, _ := registerHTTP(t, e, r, "POST", "/items", "write", false)
	invalid := op
	invalid.Arguments = map[string]any{"url": "https://elsewhere.test"}
	if _, err := e.Preview(invalid); err == nil {
		t.Fatal("extra argument accepted")
	}
	p, err := e.Preview(op)
	if err != nil {
		t.Fatal(err)
	}
	out, err := e.ExecutePreview(context.Background(), p.Token)
	if err == nil || hits.Load() != 1 || !strings.Contains(out, "outcome_unknown") {
		t.Fatal(out, err, hits.Load())
	}
}
func TestGatewayHostScopeIndependentOfTunnelAndVersionChanges(t *testing.T) {
	e, r := setup(t, "https://example.test")
	op, st := registerHTTP(t, e, r, "GET", "/health", "read", true)
	host, err := e.Store.SaveHost(catalog.Host{Name: "App", Address: "example.test", Port: 22})
	if err != nil {
		t.Fatal(err)
	}
	st.Grants[0].Subject = externals.Subject{Kind: "host", ID: host.ID}
	store := externals.NewStore(e.Store)
	st, err = store.SavePermissions(r.ID, st.Revision, st.Permissions, st.Grants, "另一个 SSH 会话授权")
	if err != nil {
		t.Fatal(err)
	}
	e.HostID = host.ID
	p, err := e.Preview(op)
	if err != nil || p.Decision.Action != "allow" {
		t.Fatal(p, err)
	}
	e.HostID = "other"
	p, err = e.Preview(op)
	if err != nil || p.Decision.Action != "deny" {
		t.Fatal(p, err)
	}
	e.HostID = host.ID
	def := st.Operations[0]
	def.Description = "changed"
	st, err = store.SaveOperation(r.ID, st.Revision, def, "接口新版本")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Preview(op); err == nil {
		t.Fatal("old revision accepted")
	}
	op.OperationRevision = st.Operations[0].Revision
	p, err = e.Preview(op)
	if err != nil || p.Decision.Action != "deny" {
		t.Fatal("old grant auto upgraded", p, err)
	}
}
func TestDiscoveryContractsAreCandidatesAndChangedSpecsDisableReviewedVersion(t *testing.T) {
	schema := `{"openapi":"3.0.3","paths":{"/items/{id}":{"get":{"summary":"Read item","parameters":[{"in":"path","name":"id","required":true,"schema":{"type":"integer","minimum":1}}]}}}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, schema) }))
	defer server.Close()
	e, r := setup(t, server.URL)
	store := externals.NewStore(e.Store)
	_, err := store.Ensure(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Explore(context.Background(), e.Store, r.ID, ""); err != nil {
		t.Fatal(err)
	}
	st, _ := store.Read(r.ID)
	if len(st.Candidates) != 1 || len(st.Operations) != 0 || len(st.Grants) != 0 {
		t.Fatal(st)
	}
	candidate := st.Candidates[0]
	candidate.Lifecycle = "active"
	st, err = store.SaveOperation(r.ID, st.Revision, candidate, "已核对")
	if err != nil {
		t.Fatal(err)
	}
	schema = strings.Replace(schema, `"minimum":1`, `"minimum":2`, 1)
	if _, err := Explore(context.Background(), e.Store, r.ID, ""); err != nil {
		t.Fatal(err)
	}
	st, _ = store.Read(r.ID)
	if st.Operations[0].Lifecycle != "draft" || len(st.Candidates) != 1 {
		t.Fatal("changed spec kept automatic approval", st)
	}
	raw, _ := json.Marshal(st.Candidates[0].InputSchema)
	if !strings.Contains(string(raw), `"minimum":2`) {
		t.Fatal(string(raw))
	}
}

func TestChangedTunnelTargetInvalidatesDefinitions(t *testing.T) {
	e, r := setup(t, "https://example.test")
	h, err := e.Store.SaveHost(catalog.Host{Name: "Gateway", Address: "gateway-a.test", Port: 22})
	if err != nil {
		t.Fatal(err)
	}
	r.Route = "ssh"
	r.HostID = h.ID
	r, err = e.Store.SaveResource(r)
	if err != nil {
		t.Fatal(err)
	}
	op, _ := registerHTTP(t, e, r, "GET", "/health", "read", true)
	if p, err := e.Preview(op); err != nil || p.Decision.Action != "allow" {
		t.Fatal(p, err)
	}
	h.Address = "gateway-b.test"
	if _, err := e.Store.SaveHost(h); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Preview(op); err == nil || !strings.Contains(err.Error(), "重新确认") {
		t.Fatal("changed network reused old contract", err)
	}
}
