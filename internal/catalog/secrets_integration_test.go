package catalog

import (
	"os"
	"testing"
)

// Opt-in because this exercises the signed-in user's actual OS credential store.
// It uses a separate service and deletes the one random test item on exit.
func TestOSKeyringRoundTrip(t *testing.T) {
	if os.Getenv("AISSH_TEST_OS_KEYRING") != "1" {
		t.Skip("set AISSH_TEST_OS_KEYRING=1 to test the real OS keyring")
	}
	store := OSSecrets{Service: "com.wrdtech.aissh.integration-test"}
	ref, value := NewID(), "test-only-"+NewID()+"-中文-\"'\n"
	t.Cleanup(func() {
		if err := store.Delete(ref); err != nil {
			t.Error(err)
		}
	})
	if err := store.Set(ref, value); err != nil {
		t.Fatal(err)
	}
	saved, err := store.Get(ref)
	if err != nil {
		t.Fatal(err)
	}
	if saved != value {
		t.Fatal("OS keyring did not preserve the value")
	}
	if err := store.Delete(ref); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ref); err == nil {
		t.Fatal("deleted keyring item still exists")
	}
}
