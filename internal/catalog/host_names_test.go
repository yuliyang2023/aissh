package catalog

import "testing"

func TestHostNamesAcrossSavePaths(t *testing.T) {
	for _, path := range []string{"host", "conversation", "saved-conversation"} {
		for _, tc := range []struct {
			name string
			host Host
			want string
		}{
			{"default port", Host{Address: "192.0.2.10", User: "ops"}, "ops@192.0.2.10:22"},
			{"custom port", Host{Address: "192.0.2.10", Port: 2222, User: "ops"}, "ops@192.0.2.10:2222"},
			{"credential user", Host{Address: "server.example", CredentialID: "login"}, "deploy@server.example:22"},
			{"explicit user", Host{Address: "server.example", User: "root", CredentialID: "login"}, "root@server.example:22"},
			{"no user", Host{Address: "server.example"}, "server.example:22"},
			{"IPv6", Host{Address: "2001:db8::10", User: "ops", Port: 2222}, "ops@[2001:db8::10]:2222"},
			{"whitespace", Host{Name: "  ", Address: " 192.0.2.10 ", User: " ops "}, "ops@192.0.2.10:22"},
			{"custom name", Host{Name: "  生产服务器  ", Address: "server.example", User: "ops"}, "生产服务器"},
		} {
			t.Run(path+"/"+tc.name, func(t *testing.T) {
				s, _ := testStore(t)
				if _, err := s.SaveCredential(CredentialInput{Credential: Credential{ID: "login", Name: "Login", Kind: "agent", Username: "deploy"}}); err != nil {
					t.Fatal(err)
				}
				var saved Host
				var err error
				if path == "host" {
					saved, err = s.SaveHost(tc.host)
				} else {
					saved, err = s.CreateConversationHost(tc.host, path == "saved-conversation")
				}
				if err != nil {
					t.Fatal(err)
				}
				if saved.Name != tc.want {
					t.Fatalf("name = %q, want %q", saved.Name, tc.want)
				}
				d, err := s.Load()
				if err != nil {
					t.Fatal(err)
				}
				loaded, err := d.Host(saved.ID)
				if err != nil || loaded != saved {
					t.Fatalf("saved host changed after reload: %+v, %v", loaded, err)
				}
				if tc.host.CredentialID != "" && loaded.User != tc.host.User {
					t.Fatal("automatic name must not override the configured SSH user")
				}
			})
		}
	}
}

func TestClearingHostNameUsesCurrentConnection(t *testing.T) {
	s, _ := testStore(t)
	h, err := s.SaveHost(Host{Name: "生产服务器", Address: "192.0.2.10", User: "ops"})
	if err != nil {
		t.Fatal(err)
	}
	h.Name, h.Port = "", 2222
	saved, err := s.SaveHost(h)
	if err != nil || saved.ID != h.ID || saved.Name != "ops@192.0.2.10:2222" {
		t.Fatalf("cleared name was not regenerated: %+v, %v", saved, err)
	}
	d, err := s.Load()
	if err != nil || len(d.Hosts) != 1 || d.Hosts[0] != saved {
		t.Fatalf("edited host was not persisted: %+v, %v", d.Hosts, err)
	}
}
