package desktop

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"git.example.com/ai/wrd-ai-cli/internal/sshauth"
)

func desktopEnvironment() []string {
	env := sshauth.CleanEnvironment(os.Environ())
	if runtime.GOOS == "windows" {
		return env
	}
	home, _ := os.UserHomeDir()
	for i, entry := range env {
		if strings.HasPrefix(entry, "PATH=") {
			env[i] = entry + ":/opt/homebrew/bin:/usr/local/bin:" + filepath.Join(home, ".local", "bin")
			return env
		}
	}
	return append(env, "PATH=/usr/bin:/bin:/usr/sbin:/sbin:/opt/homebrew/bin:/usr/local/bin:"+filepath.Join(home, ".local", "bin"))
}
