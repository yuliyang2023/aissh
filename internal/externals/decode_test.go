package externals

import (
	"encoding/json"
	"testing"
)

func TestUnsupportedScopeAndMappingAreNotSilentlyDropped(t *testing.T) {
	var grant Grant
	if err := json.Unmarshal([]byte(`{"constraints":{"key":{"prefix":"orders:","pattern":"^orders:[0-9]+$"}}}`), &grant); err == nil {
		t.Fatal("unsupported constraint silently broadened the grant")
	}
	var op Definition
	if err := json.Unmarshal([]byte(`{"request":{"method":"GET","path":"/items","headers":{"X-Tenant":"a"}}}`), &op); err == nil {
		t.Fatal("unmapped header silently dropped")
	}
	if err := json.Unmarshal([]byte(`{"request":{"method":"GET","path":"/items","parameters":[{"input":"id","in":"query","name":"id","unknown":true}]}}`), &op); err == nil {
		t.Fatal("unrecognized mapping field ignored")
	}
}
