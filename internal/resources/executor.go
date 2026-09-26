// Package resources executes typed operations against saved service bindings.
// Model-generated shell code is never given a secret or a secret-bearing file.
package resources

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/catalog"
	"git.example.com/ai/wrd-ai-cli/internal/redact"
	"git.example.com/ai/wrd-ai-cli/internal/sshauth"
)

const Prefix = "@resource "

type Operation struct {
	OperationID       string         `json:"operationId,omitempty"`
	OperationRevision uint64         `json:"operationRevision,omitempty"`
	Arguments         map[string]any `json:"arguments,omitempty"`
	ResourceID        string         `json:"resourceId"`
	Action            string         `json:"action"` // system.describe or system.invoke; raw fields are internal discovery probes
	Path              string         `json:"path,omitempty"`
	Query             string         `json:"query,omitempty"`
	Args              []string       `json:"args,omitempty"`
}

func Parse(command string) (Operation, error) {
	var op Operation
	if !strings.HasPrefix(command, Prefix) {
		return op, errors.New("不是系统连接操作")
	}
	decoder := json.NewDecoder(strings.NewReader(strings.TrimPrefix(command, Prefix)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&op); err != nil {
		return op, errors.New("系统操作格式无效")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return op, errors.New("系统操作不能包含额外命令")
	}
	if err := catalog.ValidateID(op.ResourceID); err != nil {
		return op, err
	}
	return op, nil
}
func (op Operation) Command() string { data, _ := json.Marshal(op); return Prefix + string(data) }

type Executor struct {
	Store     *catalog.Store
	HostID    string // trusted current session, never supplied by the model
	Helper    string
	Filter    *redact.Filter
	mu        sync.Mutex
	SessionID string
	pending   map[string]pendingInvocation
}

func New(store *catalog.Store, hostID, helper string, filter *redact.Filter) *Executor {
	if filter == nil {
		filter = redact.New()
	}
	return &Executor{Store: store, HostID: hostID, Helper: helper, Filter: filter, SessionID: catalog.NewID(), pending: map[string]pendingInvocation{}}
}

// RequiresApproval prepares and pins the exact request shown by the SSH UI.
func (e *Executor) RequiresApproval(op Operation) (bool, error) {
	if op.Action == "system.describe" {
		_, err := e.Describe(op.ResourceID)
		return false, err
	}
	p, err := e.Preview(op)
	if err != nil {
		return true, err
	}
	if p.Decision.Action == "deny" {
		return true, errors.New(strings.Join(p.Decision.Reasons, "；"))
	}
	return p.Decision.Action != "allow", nil
}

// The approved flag is trusted UI state, never a model argument. It is valid
// only for a still-current preview retained by this executor.
func (e *Executor) Execute(ctx context.Context, op Operation, approved bool) (string, error) {
	if op.Action == "system.describe" {
		return e.Describe(op.ResourceID)
	}
	return e.executeManaged(ctx, op, approved, "")
}

func (e *Executor) executeBound(ctx context.Context, d catalog.Data, r catalog.Resource, c catalog.Credential, op Operation) (string, error) {
	var err error
	secret := ""
	if r.CredentialID != "" {
		secret, err = e.Store.Secret(c)
		if err != nil {
			return "", err
		}
		e.Filter.Add(secret, c.Username+":"+secret)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	operationContext := ctx
	dial := func(ctx context.Context, network, address string) (net.Conn, error) {
		if r.Route == "ssh" {
			return sshauth.DialSnapshot(operationContext, e.Store, d, r.HostID, e.Helper, address, e.Filter)
		}
		return (&net.Dialer{Timeout: 8 * time.Second}).DialContext(ctx, network, address)
	}
	var output string
	switch r.Kind {
	case "http":
		output, err = executeHTTP(ctx, r, op, c, secret, dial)
	case "postgres", "mysql":
		output, err = executeSQL(ctx, r, op, c, secret, dial)
	case "redis":
		output, err = executeRedis(ctx, r, op, c, secret, dial)
	}
	if err != nil {
		if ctx.Err() != nil {
			return "", errors.New("系统操作超时或已取消")
		}
		return "", fmt.Errorf("系统操作失败：%s", e.Filter.Text(err.Error()))
	}
	return e.Filter.Text(output), nil
}

type dialFunc func(context.Context, string, string) (net.Conn, error)

func requestURL(base, path string) (*url.URL, error) {
	u, err := catalog.ServiceURL(base)
	if err != nil {
		return nil, err
	}
	if path == "" {
		path = "/"
	}
	relative, err := url.Parse(path)
	if err != nil || relative.IsAbs() || relative.Host != "" || relative.User != nil || relative.Fragment != "" || strings.ContainsAny(path, "\r\n\\") {
		return nil, errors.New("请求路径必须是当前系统内的相对路径")
	}
	for _, segment := range strings.Split(relative.Path, "/") {
		if segment == ".." {
			return nil, errors.New("请求不能离开配置的系统路径")
		}
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/" + strings.TrimLeft(relative.Path, "/")
	u.RawPath = ""
	u.RawQuery = relative.RawQuery
	return u, nil
}
func executeHTTP(ctx context.Context, r catalog.Resource, op Operation, c catalog.Credential, secret string, dial dialFunc) (string, error) {
	u, err := requestURL(r.Endpoint, op.Path)
	if err != nil {
		return "", err
	}
	transport := &http.Transport{Proxy: nil, DialContext: dial, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
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
	body, err := io.ReadAll(io.LimitReader(response.Body, 128*1024+1))
	if err != nil {
		return "", err
	}
	truncated := ""
	if len(body) > 128*1024 {
		body = body[:128*1024]
		truncated = "\n[结果已截断]"
	}
	// Do not forward Set-Cookie or other response headers to the model.
	return fmt.Sprintf("HTTP %d\n%s%s", response.StatusCode, string(body), truncated), nil
}

func (e *Executor) Descriptions() string { return e.managedDescriptions() }
