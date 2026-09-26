package resources

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"git.example.com/ai/wrd-ai-cli/internal/catalog"
	"git.example.com/ai/wrd-ai-cli/internal/externals"
)

func executeContractHTTP(ctx context.Context, r catalog.Resource, p externals.Prepared, c catalog.Credential, secret string, dial dialFunc, limit int, outputSchema externals.Schema) (string, error) {
	u, err := requestURL(r.Endpoint, p.Path)
	if err != nil {
		return "", err
	}
	transport := &http.Transport{Proxy: nil, DialContext: dial, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, p.Method, u.String(), strings.NewReader(p.Body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	if p.Body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if secret != "" {
		if c.Kind == "token" {
			req.Header.Set("Authorization", "Bearer "+secret)
		} else {
			req.SetBasicAuth(c.Username, secret)
		}
	}
	response, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, int64(limit)+1))
	if err != nil {
		return "", err
	}
	truncated := len(body) > limit
	if truncated {
		body = body[:limit]
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d（未跟随重定向；请核对服务状态）", response.StatusCode)
	}
	if len(outputSchema) > 0 {
		var value any
		if truncated || json.Unmarshal(body, &value) != nil {
			return "", fmt.Errorf("响应不是完整 JSON，无法验证输出定义")
		}
		if err := externals.ValidateOutput(outputSchema, value); err != nil {
			return "", fmt.Errorf("输出不符合接口定义：%w", err)
		}
	}
	result, _ := json.Marshal(map[string]any{"status": response.StatusCode, "body": string(body), "truncated": truncated})
	return string(result), nil
}
