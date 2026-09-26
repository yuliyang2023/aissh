package catalog

import "testing"

func TestConversationTargetProtectsBoundConfiguration(t *testing.T) {
	s, _ := testStore(t)
	credential, err := s.SaveCredential(CredentialInput{Credential: Credential{Name: "login", Kind: "agent"}})
	if err != nil {
		t.Fatal(err)
	}
	jump, err := s.SaveHost(Host{Name: "jump", Address: "jump.example"})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := s.SaveAIProfile(AIProfile{Name: "assistant", Provider: "codex"}, false)
	if err != nil {
		t.Fatal(err)
	}
	h, err := s.CreateConversationHost(Host{Address: "private.example", CredentialID: credential.ID, JumpHostID: jump.ID, AIProfileID: profile.ID}, false)
	if err != nil {
		t.Fatal(err)
	}
	for kind, id := range map[string]string{"credential": credential.ID, "host": jump.ID, "ai": profile.ID} {
		if err := s.Delete(kind, id); err == nil {
			t.Fatalf("deleted referenced %s", kind)
		}
	}
	if err := s.RecordConnection(h.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("host", h.ID); err != nil {
		t.Fatal(err)
	}
	d, err := s.Load()
	if err != nil || len(d.ConversationHosts) != 0 || len(d.RecentConnections) != 0 {
		t.Fatal("deleted target retained", err)
	}
	if err := s.Delete("credential", credential.ID); err != nil {
		t.Fatal(err)
	}
}
