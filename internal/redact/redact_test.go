package redact

import (
	"strings"
	"testing"
)

func TestFilterScrubsWholeAndEncodedSecrets(t *testing.T) {
	f := New("secret/with space")
	out := f.Text("secret/with space secret%2Fwith+space c2VjcmV0L3dpdGggc3BhY2U=")
	if strings.Count(out, "[redacted]") != 3 {
		t.Fatal(out)
	}
	var empty *Filter
	if empty.Text("safe") != "safe" {
		t.Fatal("nil filter changed text")
	}
}
