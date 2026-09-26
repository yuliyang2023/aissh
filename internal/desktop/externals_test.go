package desktop

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.example.com/ai/wrd-ai-cli/internal/catalog"
	"git.example.com/ai/wrd-ai-cli/internal/externals"
	"git.example.com/ai/wrd-ai-cli/internal/resources"
)

func TestDesktopDoesNotBorrowSSHIdentityOrExecuteSimulations(t *testing.T) {
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++; fmt.Fprint(w, "ok") }))
	defer server.Close()
	store := catalog.New(t.TempDir(), memorySecrets{})
	s := New(store)
	host, err := store.SaveHost(catalog.Host{Name: "Host", Address: "example.test", Port: 22})
	if err != nil {
		t.Fatal(err)
	}
	r, err := store.SaveResource(catalog.Resource{Name: "Service", Kind: "http", Endpoint: server.URL, HostID: host.ID})
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.GetExternalState(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	def := externals.Definition{ID: "status", Name: "Status", Kind: "http", Lifecycle: "active", InputSchema: externals.EmptyInput(), Request: externals.Request{Method: "GET", Path: "/"}, Assessment: externals.Assessment{Effects: externals.Effects{Access: "read"}, Risk: "low"}}
	st, err := s.SaveExternalOperation(r.ID, v.State.Revision, def, "review")
	if err != nil {
		t.Fatal(err)
	}
	g := externals.Grant{Name: "SSH reader", Subject: externals.Subject{Kind: "host", ID: host.ID}, Operations: map[string]uint64{def.ID: 1}, Permissions: []string{"read"}, AutoRead: true}
	st, err = s.SaveExternalPermissions(r.ID, st.Revision, st.Permissions, []externals.Grant{g}, "scope")
	if err != nil {
		t.Fatal(err)
	}
	op := resources.Operation{ResourceID: r.ID, Action: "system.invoke", OperationID: def.ID, OperationRevision: 1, Arguments: map[string]any{}}
	p, err := s.PreviewExternalCall(op)
	if err != nil || p.Decision.Action != "deny" {
		t.Fatal("desktop impersonated SSH", p, err)
	}
	if _, err := s.InvokeExternalCall(p.Token); err == nil || hits != 0 {
		t.Fatal("denied call executed", err, hits)
	}
	simulated, err := s.SimulateExternalCall(op, externals.Caller{HostID: host.ID})
	if err != nil || simulated.Decision.Action != "allow" || simulated.Token != "" {
		t.Fatal(simulated, err)
	}
	if _, err := s.InvokeExternalCall(simulated.Token); err == nil || hits != 0 {
		t.Fatal("simulation became authorization")
	}
	if _, err := s.RunResource(op); err == nil {
		t.Fatal("legacy RunResource bypassed preview")
	}
	g.Subject = externals.Subject{Kind: "local"}
	_, err = s.SaveExternalPermissions(r.ID, st.Revision, st.Permissions, []externals.Grant{g}, "local reader")
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.PreviewExternalCall(op)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.InvokeExternalCall(p.Token); err != nil || hits != 1 {
		t.Fatal(err, hits)
	}
	if _, err := s.InvokeExternalCall(p.Token); err == nil || hits != 1 {
		t.Fatal("desktop token replayed")
	}
}
