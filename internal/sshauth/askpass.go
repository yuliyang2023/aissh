// Package sshauth connects managed profiles to the system OpenSSH client.
package sshauth

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/catalog"
	"git.example.com/ai/wrd-ai-cli/internal/redact"
)

const askpassAddress = "AISSH_ASKPASS_ADDRESS"
const askpassToken = "AISSH_ASKPASS_CAPABILITY"

type answer struct {
	prompts    []string
	credential catalog.Credential
	attempts   int
}
type broker struct {
	mu      sync.Mutex
	answers []answer
	store   *catalog.Store
	filter  *redact.Filter
	token   string
}

func (b *broker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost || r.URL.Path != "/answer" || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+b.token)) != 1 {
		http.Error(w, "denied", http.StatusForbidden)
		return
	}
	var request struct {
		Prompt string `json:"prompt"`
		Hint   string `json:"hint"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&request) != nil || request.Hint == "confirm" {
		http.Error(w, "unsupported prompt", http.StatusForbidden)
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	matchedID := ""
	for _, a := range b.answers {
		for _, p := range a.prompts {
			if strings.TrimSpace(request.Prompt) == p {
				if matchedID != "" && matchedID != a.credential.ID {
					http.Error(w, "ambiguous identity prompt", http.StatusForbidden)
					return
				}
				matchedID = a.credential.ID
			}
		}
	}
	for i := range b.answers {
		a := &b.answers[i]
		for _, p := range a.prompts {
			if strings.TrimSpace(request.Prompt) != p {
				continue
			}
			if a.attempts >= 1 {
				// A jump and destination can legitimately share the same key
				// prompt. Each bound hop gets one attempt for that credential.
				continue
			}
			a.attempts++
			value, err := b.store.Secret(a.credential)
			if err != nil {
				http.Error(w, "unlock or update the saved credential", http.StatusForbidden)
				return
			}
			b.filter.Add(value)
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = io.WriteString(w, value)
			return
		}
	}
	http.Error(w, "authentication requires manual input; this prompt is not bound to a saved credential", http.StatusForbidden)
}

// RunHelper only talks to the per-session loopback broker. It never looks up a
// vault item based on a prompt supplied by a remote host.
func RunHelper(prompt string, output io.Writer) error {
	address, token := os.Getenv(askpassAddress), os.Getenv(askpassToken)
	host, port, err := net.SplitHostPort(address)
	if err != nil || host != "127.0.0.1" || len(token) != 32 {
		return errors.New("invalid askpass session")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return errors.New("invalid askpass endpoint")
	}
	body, _ := json.Marshal(map[string]string{"prompt": prompt, "hint": os.Getenv("SSH_ASKPASS_PROMPT")})
	ctx, cancel := context.WithTimeout(context.Background(), 190*time.Second)
	defer cancel()
	r, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+address+"/answer", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	resp, err := client.Do(r)
	if err != nil {
		return errors.New("credential session unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("saved credential is unavailable or does not match this authentication request")
	}
	value, err := io.ReadAll(io.LimitReader(resp.Body, 2049))
	if err != nil || len(value) > 2048 {
		return errors.New("invalid credential response")
	}
	_, err = output.Write(value)
	return err
}

// CleanEnvironment prevents unrelated API credentials and previous session
// capabilities from being inherited by SSH. SSH_AUTH_SOCK is intentionally kept.
func CleanEnvironment(env []string) []string {
	result := make([]string, 0, len(env))
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		upper := strings.ToUpper(key)
		if strings.HasSuffix(upper, "_API_KEY") || strings.HasSuffix(upper, "_TOKEN") || strings.HasSuffix(upper, "_PASSWORD") || strings.HasSuffix(upper, "_SECRET") || strings.HasPrefix(upper, "AISSH_ASKPASS_") || strings.HasPrefix(upper, "AISSH_DESKTOP_") || upper == "SSH_ASKPASS" || upper == "SSH_ASKPASS_REQUIRE" {
			continue
		}
		result = append(result, entry)
	}
	return result
}
