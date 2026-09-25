package agent

import (
	"os"
	"path/filepath"
	"strings"

	"git.example.com/ai/wrd-ai-cli/internal/sshauth"
)

// Keep the selected CLI's own conventional login variable for compatibility,
// while removing unrelated service secrets and desktop/askpass capabilities.
func localProviderEnvironment(provider string, executable ...string) []string {
	env := sshauth.CleanEnvironment(os.Environ())
	// npm-installed CLIs may use /usr/bin/env node. Include the selected
	// executable's directory so desktop launches can find the matching runtime.
	if len(executable) > 0 && filepath.IsAbs(executable[0]) {
		for i, entry := range env {
			if strings.HasPrefix(entry, "PATH=") {
				env[i] = "PATH=" + filepath.Dir(executable[0]) + string(os.PathListSeparator) + strings.TrimPrefix(entry, "PATH=")
				break
			}
		}
	}
	allowed := "OPENAI_API_KEY"
	if provider == "claude" {
		allowed = "ANTHROPIC_API_KEY"
	}
	if value, ok := os.LookupEnv(allowed); ok {
		env = append(env, allowed+"="+value)
	}
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "CLAUDE_CODE_OAUTH_TOKEN=") && provider == "claude" {
			env = append(env, entry)
		}
	}
	return env
}
