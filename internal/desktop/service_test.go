package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/catalog"
)

type memorySecrets map[string]string

func (m memorySecrets) Get(k string) (string, error) {
	v, ok := m[k]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}
func (m memorySecrets) Set(k, v string) error { m[k] = v; return nil }
func (m memorySecrets) Delete(k string) error { delete(m, k); return nil }
func TestDesktopReadsNeverExposeSecrets(t *testing.T) {
	s := New(catalog.New(t.TempDir(), memorySecrets{}))
	c, err := s.SaveCredential(catalog.CredentialInput{Credential: catalog.Credential{Name: "test", Kind: "token"}, Secret: "desktop-test-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if c.SecretRef != "" {
		t.Fatal("frontend received keyring locator")
	}
	d, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(d)
	if strings.Contains(string(data), "desktop-test-secret") || strings.Contains(string(data), "secretRef") {
		t.Fatal("frontend read exposed secret")
	}
}
func TestCredentialRepairDecisionResumesRequest(t *testing.T) {
	s := New(catalog.New(t.TempDir(), memorySecrets{}))
	events := make(chan credentialRequest, 1)
	s.emit = func(name string, args ...any) {
		if name == "credential-needed" {
			events <- args[0].(credentialRequest)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.requestCredential(ctx, "session", "credential") }()
	request := <-events
	if request.CredentialID != "credential" {
		t.Fatal("wrong credential requested")
	}
	if err := s.ResolveCredentialRequest(request.RequestID, true); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := s.ResolveCredentialRequest(request.RequestID, true); err == nil {
		t.Fatal("request can be replayed")
	}
}
