package catalog

import (
	"fmt"
	"testing"
)

func TestRecentConnectionsPersistBoundAndClear(t *testing.T) {
	s, _ := testStore(t)
	var first Host
	for i := 0; i < 51; i++ {
		h, err := s.SaveHost(Host{Name: fmt.Sprintf("host-%d", i), Address: "localhost", Port: 22})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = h
		}
		if err := s.RecordConnection(h.ID); err != nil {
			t.Fatal(err)
		}
	}
	reloaded := New(s.Dir, s.Secrets)
	d, err := reloaded.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(d.RecentConnections) != 50 || d.RecentConnections[0].OpenedAt == "" {
		t.Fatal("history not persisted or bounded")
	}
	for _, r := range d.RecentConnections {
		if r.HostID == first.ID {
			t.Fatal("oldest connection retained beyond limit")
		}
	}
	newest := d.RecentConnections[0].HostID
	revision := d.Revision
	if err := reloaded.RecordConnection(newest); err != nil {
		t.Fatal(err)
	}
	d, _ = reloaded.Load()
	if len(d.RecentConnections) != 50 || d.RecentConnections[0].HostID != newest {
		t.Fatal("duplicate recent connection")
	}
	if d.Revision != revision {
		t.Fatal("opening a session invalidated resource grants")
	}
	if err := reloaded.Delete("host", newest); err != nil {
		t.Fatal(err)
	}
	d, _ = reloaded.Load()
	if len(d.RecentConnections) != 49 {
		t.Fatal("deleted host retained in history")
	}
	if err := reloaded.RecordConnection("missing"); err == nil {
		t.Fatal("unknown host added to history")
	}
	revision = d.Revision
	if err := reloaded.ClearRecentConnections(); err != nil {
		t.Fatal(err)
	}
	d, _ = reloaded.Load()
	if len(d.RecentConnections) != 0 || len(d.Hosts) != 50 {
		t.Fatal("clearing history removed host configuration or left history")
	}
	if d.Revision != revision {
		t.Fatal("clearing history invalidated resource grants")
	}
}
