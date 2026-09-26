package desktop

import (
	"fmt"
	"git.example.com/ai/wrd-ai-cli/internal/catalog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestSavingResourceExploresAndKeepsFailureSeparate(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/spec.json" {
			t.Error("unexpected probe", r.URL)
		}
		fmt.Fprint(w, `{"openapi":"3.0.3","paths":{"/health":{"get":{"summary":"Health status"}}}}`)
	}))
	s := New(catalog.New(t.TempDir(), memorySecrets{}))
	r, err := s.SaveResource(catalog.Resource{Name: "Service", Kind: "http", Endpoint: server.URL, DiscoveryPath: "/spec.json"})
	if err != nil || !r.Managed {
		t.Fatal(r, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		view, err := s.GetExternalState(r.ID)
		if err != nil {
			t.Fatal(err)
		}
		if view.Job.Status != "running" {
			if view.Discovery == nil || view.Discovery.Status != "ready" || len(view.State.Candidates) != 1 {
				t.Fatal(view)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("background discovery timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
	r.Name = "Renamed"
	r, err = s.SaveResource(r)
	if err != nil || hits.Load() != 1 {
		t.Fatal("name edit unnecessarily reprobed", err, hits.Load())
	}
	server.Close()
	report, err := s.ExploreResource(r.ID)
	if err != nil || len(report.Notes) == 0 {
		t.Fatal("failure was not reported", report, err)
	}
	d, err := s.Snapshot()
	if err != nil || len(d.Resources) != 1 || d.Resources[0].Name != "Renamed" {
		t.Fatal("failed discovery lost saved connection", err)
	}
}

func TestSnapshotMarksRotatedCredentialGuideStale(t *testing.T) {
	s := New(catalog.New(t.TempDir(), memorySecrets{}))
	c, err := s.store.SaveCredential(catalog.CredentialInput{Credential: catalog.Credential{Name: "Token", Kind: "token"}, Secret: "first"})
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.store.SaveResource(catalog.Resource{Name: "Service", Kind: "http", Endpoint: "https://example.test", CredentialID: c.ID})
	if err != nil {
		t.Fatal(err)
	}
	d, _ := s.store.Load()
	if err := s.store.SaveResourceDiscovery(r.ID, catalog.ResourceDiscovery{Binding: catalog.ResourceBinding(r, d), Status: "ready"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.store.SaveCredential(catalog.CredentialInput{Credential: c, Secret: "second"}); err != nil {
		t.Fatal(err)
	}
	d, err = s.Snapshot()
	if err != nil || d.Resources[0].Discovery.Status != "stale" {
		t.Fatal("old guide presented as current", err)
	}
}
