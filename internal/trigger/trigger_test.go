package trigger

import "testing"

func TestAIInput(t *testing.T) {
	for _, input := range []string{"帮我检查 nginx", "  /ai explain this"} {
		if _, ok := IsAIInput(input); !ok {
			t.Fatalf("expected AI input: %q", input)
		}
	}
	if _, ok := IsAIInput("echo 中文"); ok {
		t.Fatal("shell command should not be classified as AI input")
	}
}

func TestPromptAndError(t *testing.T) {
	if !LooksLikePrompt("root@host:~# ") {
		t.Fatal("expected prompt")
	}
	if LooksLikePrompt("Password: ") {
		t.Fatal("password prompt must not be treated as shell prompt")
	}
	match := MatchError("bash: docker-compose: command not found\r\n")
	if match == nil || match.Kind != "command_not_found" {
		t.Fatalf("match = %#v", match)
	}
	if match.Message != "bash: docker-compose: command not found" {
		t.Fatalf("message = %q", match.Message)
	}
}
