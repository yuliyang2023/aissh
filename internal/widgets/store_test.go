package widgets

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestSavedVersionsAndTraversal(t *testing.T) {
	dir := t.TempDir()
	d := Definition{ID: "p", Title: "Example", Command: "uptime", Contract: Contract{Format: "text", Fields: []Field{{Name: "text", Type: "string"}}}, View: View{Kind: "custom", SDKVersion: 1, StateVersion: 1, Source: "valid source"}}
	for i := 1; i <= 12; i++ {
		d.Revision = uint64(i)
		d.Title = fmt.Sprint(i)
		if err := Save(dir, "host", d); err != nil {
			t.Fatal(err)
		}
	}
	all, err := ListSaved(dir, "host")
	if err != nil || len(all) != 1 || len(all[0].Versions) != 10 || all[0].Versions[0].Version != 3 {
		t.Fatalf("%v %v", all, err)
	}
	if list, _ := ListSaved(dir, "other-host"); len(list) != 0 {
		t.Fatal("cross-host disclosure")
	}
	if _, err := LoadSaved(dir, "../../anything"); err == nil {
		t.Fatal("path accepted")
	}
}

func TestRedactedSnapshotKeepsOriginalAndMarksMissing(t *testing.T) {
	original := Data{Data: map[string]any{"label": "secret-value", "n": float64(4)}, LastGood: map[string]any{"label": "secret-value"}, Error: "failed secret-value"}
	clean := Redact(original, func(s string) string { return strings.ReplaceAll(s, "secret-value", "[redacted]") })
	if clean.Data["label"] != nil || clean.LastGood["label"] != nil || clean.Data["n"] != float64(4) || len(clean.Issues) != 2 || strings.Contains(clean.Error, "secret-value") {
		t.Fatalf("invalid redacted snapshot: %#v", clean)
	}
	if original.Data["label"] != "secret-value" || len(original.Issues) != 0 {
		t.Fatal("redaction mutated the collector's authoritative sample")
	}
}
func TestHistoryGapAndBound(t *testing.T) {
	c := Contract{Format: "kv", Fields: []Field{{Name: "n", Type: "number", History: true}}}
	d := Data{BindingID: "b", IntervalMS: 1000}
	now := time.Now()
	d = Record(d, c, "n=4", nil, now)
	d = Record(d, c, "", errors.New("offline"), now.Add(time.Second))
	if d.Data != nil || d.History[1].Values["n"] != nil {
		t.Fatal("error became numeric sample")
	}
	d = Record(d, c, "n=5", nil, now.Add(time.Minute))
	if len(d.History) != 4 {
		t.Fatal("missing stale gap")
	}
	for i := 0; i < 800; i++ {
		d = Record(d, c, "n=5", nil, now.Add(time.Duration(i+61)*time.Second))
	}
	if len(d.History) > 600 || !d.Truncated {
		t.Fatal("unbounded history")
	}
}
