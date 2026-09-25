// Package credentialui lets a running CLI ask its owning desktop window to
// repair a missing credential. This channel carries IDs and decisions only.
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
	address, token := os.Getenv(AddressEnv), os.Getenv(CapabilityEnv)
	host, _, err := net.SplitHostPort(address)
	if err != nil || host != "127.0.0.1" || len(token) != 32 {
		return errors.New("请在桌面端更新凭据后重试")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	body, _ := json.Marshal(map[string]string{"credentialId": id})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+address+"/credential", strings.NewReader(string(body)))
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
