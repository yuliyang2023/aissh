package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.example.com/ai/wrd-ai-cli/internal/catalog"
)

const testAPI = `{"openapi":"3.0.3","paths":{
 "/assets":{"get":{"summary":"List assets","parameters":[{"name":"limit","in":"query"}]},"post":{"summary":"Create asset"}},
 "/assets/{id}":{"parameters":[{"name":"id","in":"path","required":true}],"get":{"summary":"Read asset"},"delete":{"summary":"Delete asset"}},
 "/restart":{"get":{"summary":"Restart service"}},
 "/action":{"get":{"summary":"Custom action"}},
 "/health":{"get":{"summary":"Health status"}}
}}`

func TestDiscoveryReadsOnlyDocumentsAndPersistsCapabilities(t *testing.T) {
	hits := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		hits = append(hits, req.Method+" "+req.URL.Path)
		if req.URL.Path != "/api/openapi.json" {
			t.Error("discovery requested business endpoint", req.URL)
			http.Error(w, "unexpected", 500)
			return
		}
		if req.Header.Get("Authorization") != "Bearer private-test-token" {
			t.Error("missing discovery credential")
		}
		fmt.Fprint(w, strings.Replace(testAPI, "List assets", "List assets private-test-token", 1))
	}))
	defer server.Close()
	e, r := setup(t, server.URL+"/api")
	report, err := Explore(context.Background(), e.Store, r.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "ready" || len(report.Operations) != 7 {
		t.Fatalf("report: %+v", report)
	}
	if len(hits) != 1 || hits[0] != "GET /api/openapi.json" {
		t.Fatal(hits)
	}
	raw, _ := json.Marshal(report)
	if strings.Contains(string(raw), "private-test-token") {
		t.Fatal("secret persisted")
	}
	risks := map[string]string{}
	for _, op := range report.Operations {
		risks[op.Method+" "+op.Path] = op.Risk
	}
	for key, want := range map[string]string{"GET /assets": "read", "POST /assets": "write", "DELETE /assets/{id}": "dangerous", "GET /restart": "dangerous", "GET /action": "unknown"} {
		if risks[key] != want {
			t.Errorf("%s: %s want %s", key, risks[key], want)
		}
	}
	d, _ := e.Store.Load()
	saved, _ := d.Resource(r.ID)
	if catalog.CurrentResourceDiscovery(saved, d) == nil {
		t.Fatal("guide missing after reload")
	}
	if need, err := e.RequiresApproval(Operation{ResourceID: r.ID, Action: "http.get", Path: "/health"}); err == nil || !need {
		t.Fatal("raw operation accepted after discovery", err)
	}
	out, err := e.Execute(context.Background(), Operation{ResourceID: r.ID, Action: "system.describe"}, false)
	if err != nil || !strings.Contains(out, "List assets") || len(hits) != 1 {
		t.Fatal("guide should be local metadata", out, err)
	}
	if !strings.Contains(e.Descriptions(), "system.describe") || !strings.Contains(e.Descriptions(), "candidateCount") {
		t.Fatal("SSH missing guide")
	}
}

func TestDiscoveredRisksEnforcedAtExecution(t *testing.T) {
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path == "/openapi.json" {
			fmt.Fprint(w, testAPI)
		} else {
			fmt.Fprint(w, "ok")
		}
	}))
	defer server.Close()
	e, r := setup(t, server.URL)
	r.Grant = "read"
	r, _ = e.Store.SaveResource(r)
	if _, err := Explore(context.Background(), e.Store, r.ID, ""); err != nil {
		t.Fatal(err)
	}
	before := hits
	for _, p := range []string{"/restart", "/%72estart", "/assets?action=delete"} {
		op := Operation{ResourceID: r.ID, Action: "http.get", Path: p}
		if _, err := e.Execute(context.Background(), op, true); err == nil {
			t.Error("dangerous GET accepted", p)
		}
	}
	if hits != before {
		t.Fatal("blocked request reached server")
	}
	for _, p := range []string{"/action", "/undocumented", "/assets?limit=5", "/health", "/assets", "/assets/123"} {
		op := Operation{ResourceID: r.ID, Action: "http.get", Path: p}
		if _, err := e.Execute(context.Background(), op, true); err == nil {
			t.Fatal("discovery or legacy grant authorized unregistered business call", p)
		}
	}
	if hits != before {
		t.Fatal("unregistered operation reached server")
	}
	e.HostID = "other-host"
	if _, err := e.Execute(context.Background(), Operation{ResourceID: r.ID, Action: "system.describe"}, false); err == nil {
		t.Fatal("cross-host guide access")
	}
}

func TestDiscoveryFailuresAndCredentialChanges(t *testing.T) {
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.Error(w, "secret body should not persist", http.StatusUnauthorized)
	}))
	defer server.Close()
	e, r := setup(t, server.URL)
	report, err := Explore(context.Background(), e.Store, r.ID, "")
	if err != nil || report.Status != "partial" || len(report.Operations) != 0 || hits != 4 {
		t.Fatal(report, err, hits)
	}
	if strings.Contains(strings.Join(report.Notes, " "), "secret body") {
		t.Fatal("persisted raw body")
	}
	r.DiscoveryPath = "/restart"
	r, _ = e.Store.SaveResource(r)
	_, err = Explore(context.Background(), e.Store, r.ID, "")
	if err != nil || hits != 4 {
		t.Fatal("unsafe custom documentation path requested", err, hits)
	}
	r.DiscoveryPath = "https://other.test/schema.json"
	r, _ = e.Store.SaveResource(r)
	_, err = Explore(context.Background(), e.Store, r.ID, "")
	if err != nil || hits != 4 {
		t.Fatal("cross-origin documentation requested", err, hits)
	}
	d, _ := e.Store.Load()
	r, _ = d.Resource(r.ID)
	c, _ := d.Credential(r.CredentialID)
	_, err = e.Store.SaveCredential(catalog.CredentialInput{Credential: c, Secret: "rotated-token"})
	if err != nil {
		t.Fatal(err)
	}
	d, _ = e.Store.Load()
	if catalog.CurrentResourceDiscovery(r, d) != nil {
		t.Fatal("guide survived credential change")
	}
	if err := e.Store.SaveResourceDiscovery(r.ID, *r.Discovery); err == nil {
		t.Fatal("stale discovery overwrote new binding")
	}
	r.Discovery = &catalog.ResourceDiscovery{Status: "ready", Operations: []catalog.ResourceCapability{{Name: "forged"}}}
	r, _ = e.Store.SaveResource(r)
	if r.Discovery != nil && len(r.Discovery.Operations) > 0 {
		t.Fatal("frontend forged report")
	}
}

func TestOpenAPIScopeAndConservativeClassification(t *testing.T) {
	r := catalog.Resource{ID: "id", Kind: "http", Endpoint: "https://service.test/api"}
	for _, body := range []string{
		`{"openapi":"3.0.0","servers":[{"url":"https://elsewhere.test"}],"paths":{"/users":{"get":{"summary":"List users"}}}}`,
		`{"swagger":"2.0","basePath":"/outside","paths":{"/users":{"get":{"summary":"List users"}}}}`,
	} {
		if _, _, err := discoverOpenAPI(r, "/openapi.json", body); err == nil {
			t.Error("out-of-scope server accepted")
		}
	}
	body := `{"openapi":"3.0.0","servers":[{"url":"/api/v1"}],"paths":{"/assets":{"get":{"operationId":"listAssets"}},"/resetCache":{"get":{"operationId":"getResetCache"}},"/users":{"get":{"summary":"List users","parameters":[{"$ref":"#/components/parameters/User"}]}}}}`
	ops, _, err := discoverOpenAPI(r, "/openapi.json", body)
	if err != nil || len(ops) != 3 {
		t.Fatal(ops, err)
	}
	if ops[0].Path != "/v1/assets" || ops[0].Risk != "read" || ops[1].Risk != "dangerous" || ops[2].Risk != "unknown" {
		t.Fatal(ops)
	}
	for query, want := range map[string]string{"SELECT id FROM assets LIMIT 10": "read", "SELECT COUNT(*) FROM assets": "read", "SELECT dangerous_function()": "unknown", "SELECT 1; DELETE FROM assets": "dangerous", "SELECT 1 /* more */": "unknown"} {
		if risk := sqlRisk(query); risk != want {
			t.Error(query, risk, want)
		}
	}
}
