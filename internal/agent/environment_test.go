package agent

import (
	"strings"
	"testing"
)

func TestCLIOnlyInheritsItsOwnAuthentication(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "own-codex-auth")
	t.Setenv("ANTHROPIC_API_KEY", "own-claude-auth")
	t.Setenv("AISSH_MODEL_API_KEY", "unrelated-secret")
	t.Setenv("AISSH_DESKTOP_CAPABILITY", "desktop-authority")
	t.Setenv("AISSH_ASKPASS_CAPABILITY", "ssh-authority")
	for _, provider := range []string{"codex", "claude"} {
		env := strings.Join(localProviderEnvironment(provider), "\n")
		if strings.Contains(env, "unrelated-secret") || strings.Contains(env, "desktop-authority") || strings.Contains(env, "ssh-authority") {
			t.Fatal("unrelated credential inherited")
		}
		if provider == "codex" && (!strings.Contains(env, "own-codex-auth") || strings.Contains(env, "own-claude-auth")) {
			t.Fatal("wrong CLI auth")
		}
		if provider == "claude" && (!strings.Contains(env, "own-claude-auth") || strings.Contains(env, "own-codex-auth")) {
			t.Fatal("wrong CLI auth")
		}
	}
}
