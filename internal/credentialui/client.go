// Package credentialui lets a running CLI ask its owning desktop window to
// repair a missing credential and share its AI conversation and monitoring state.
// Each session uses an authenticated loopback channel; stored credentials are not exported.
package credentialui

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const AddressEnv = "AISSH_DESKTOP_ADDRESS"
const CapabilityEnv = "AISSH_DESKTOP_CAPABILITY"

func Available() bool { return os.Getenv(AddressEnv) != "" && os.Getenv(CapabilityEnv) != "" }
func Request(id string) error {
	return post("/credential", map[string]string{"credentialId": id}, 3*time.Minute)
}

// NotifyConversation keeps the owning desktop's terminal association current
// when a user switches conversations inside the CLI. It carries no chat text.
func NotifyConversation(id string) {
	_ = post("/conversation", map[string]string{"conversationId": id}, 2*time.Second)
}

func post(path string, input any, timeout time.Duration) error {
	address, token := os.Getenv(AddressEnv), os.Getenv(CapabilityEnv)
	host, _, err := net.SplitHostPort(address)
	if err != nil || host != "127.0.0.1" || len(token) != 32 {
		return errors.New("请在桌面端更新凭据后重试")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	body, _ := json.Marshal(input)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+address+path, strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("凭据录入窗口已关闭或超时")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return errors.New("用户取消了凭据更新")
	}
	return nil
}
