package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/panel"
)

type Target struct {
	Type      string `json:"type"`
	HostLabel string `json:"hostLabel"`
	Protocol  string `json:"protocol"`
}

type Request struct {
	Event         string            `json:"event"`
	SessionID     string            `json:"sessionId,omitempty"`
	ProposalID    string            `json:"proposalId,omitempty"`
	Prompt        string            `json:"prompt,omitempty"`
	RecentOutput  string            `json:"recentOutput,omitempty"`
	Output        string            `json:"output,omitempty"`
	Completion    string            `json:"completion,omitempty"`
	SystemContext map[string]string `json:"systemContext,omitempty"`
	Target        *Target           `json:"target,omitempty"`
}

type Session struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type Proposal struct {
	ID             string `json:"id"`
	Command        string `json:"command"`
	Explanation    string `json:"explanation"`
	ExpectedEffect string `json:"expectedEffect,omitempty"`
	Risk           string `json:"risk"`
}

type Turn struct {
	Type     string    `json:"type"`
	Session  Session   `json:"session"`
	Proposal *Proposal `json:"proposal,omitempty"`
	Message  string    `json:"message,omitempty"`
	Error    string    `json:"error,omitempty"`
}

type PanelSpec struct {
	Purpose     string                `json:"purpose,omitempty"`
	Alert       *panel.AlertCondition `json:"alert,omitempty"`
	ID          string                `json:"id,omitempty"`
	Type        string                `json:"type"`
	Title       string                `json:"title"`
	Collector   string                `json:"collector,omitempty"`
	Command     string                `json:"command,omitempty"`
	IntervalMS  int                   `json:"intervalMs,omitempty"`
	Content     string                `json:"content,omitempty"`
	AllowAIRead bool                  `json:"allowAIRead,omitempty"`
}

type PanelAction struct {
	Action string    `json:"action"`
	Panel  PanelSpec `json:"panel"`
}

type Callbacks struct {
	OnStatus      func(string)
	OnThought     func(string)
	OnAnswer      func(string)
	OnPanelAction func(PanelAction)
	OnActivity    func(Activity)
}

// Activity is a replaceable progress item, identified across start/output/end.
type Activity struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type Client struct {
	StreamURL string
	ActionURL string
	Token     string
	HTTP      *http.Client
}

func NewClient(streamURL, token string) *Client {
	streamURL = strings.TrimSpace(streamURL)
	actionURL := strings.TrimSuffix(streamURL, "/stream")
	return &Client{
		StreamURL: streamURL,
		ActionURL: actionURL,
		Token:     strings.TrimSpace(token),
		HTTP:      &http.Client{Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
}

func (c *Client) Name() string { return "HTTP" }

func (c *Client) Configured() bool { return c != nil && c.StreamURL != "" }

func (c *Client) Stream(ctx context.Context, request Request, callbacks Callbacks) (Turn, error) {
	if !c.Configured() {
		return Turn{}, fmt.Errorf("AI endpoint is not configured; set AISSH_AGENT_URL")
	}
	body, err := json.Marshal(request)
	if err != nil {
		return Turn{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.StreamURL, bytes.NewReader(body))
	if err != nil {
		return Turn{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	response, err := c.HTTP.Do(req)
	if err != nil {
		return Turn{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
		return Turn{}, fmt.Errorf("AI endpoint returned %s: %s", response.Status, strings.TrimSpace(string(data)))
	}
	return readSSE(response.Body, callbacks)
}

// AssessRisk asks an HTTP Agent for a fresh risk-only session. It uses the
// existing start event so compatible agents need no protocol extension.
func (c *Client) AssessRisk(ctx context.Context, request RiskAssessmentRequest) (RiskAssessment, error) {
	turn, err := c.Stream(ctx, Request{
		Event:  "start",
		Prompt: buildRiskAssessmentPrompt(request),
		Target: request.Target,
	}, Callbacks{})
	if err != nil {
		return RiskAssessment{}, err
	}
	return riskAssessmentFromResponse(codexResponse{
		Type:     turn.Type,
		Message:  turn.Message,
		Proposal: turn.Proposal,
	}, request.Proposal.Command)
}

func (c *Client) Action(ctx context.Context, request Request) (Turn, error) {
	if !c.Configured() {
		return Turn{}, fmt.Errorf("AI endpoint is not configured; set AISSH_AGENT_URL")
	}
	body, err := json.Marshal(request)
	if err != nil {
		return Turn{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.ActionURL, bytes.NewReader(body))
	if err != nil {
		return Turn{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	response, err := c.HTTP.Do(req)
	if err != nil {
		return Turn{}, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024))
	if err != nil {
		return Turn{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Turn{}, fmt.Errorf("AI endpoint returned %s: %s", response.Status, strings.TrimSpace(string(data)))
	}
	var envelope struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    Turn   `json:"data"`
	}
	if err := json.Unmarshal(data, &envelope); err == nil && (envelope.Success || envelope.Data.Type != "") {
		if !envelope.Success && envelope.Message != "" {
			return Turn{}, fmt.Errorf("%s", envelope.Message)
		}
		return envelope.Data, nil
	}
	var turn Turn
	if err := json.Unmarshal(data, &turn); err != nil {
		return Turn{}, err
	}
	return turn, nil
}

func readSSE(reader io.Reader, callbacks Callbacks) (Turn, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	var event string
	var dataLines []string
	var turn *Turn
	flush := func() error {
		if event == "" || len(dataLines) == 0 || turn != nil {
			event = ""
			dataLines = dataLines[:0]
			return nil
		}
		raw := strings.Join(dataLines, "\n")
		switch event {
		case "error":
			var payload struct {
				Message string `json:"message"`
			}
			_ = json.Unmarshal([]byte(raw), &payload)
			return fmt.Errorf("%s", firstNonEmpty(payload.Message, "AI stream failed"))
		case "status":
			var payload struct {
				Phase string `json:"phase"`
			}
			_ = json.Unmarshal([]byte(raw), &payload)
			if callbacks.OnStatus != nil && payload.Phase != "" {
				callbacks.OnStatus(payload.Phase)
			}
		case "thought_delta":
			var payload struct {
				Content string `json:"content"`
			}
			_ = json.Unmarshal([]byte(raw), &payload)
			if callbacks.OnThought != nil && payload.Content != "" {
				callbacks.OnThought(payload.Content)
			}
		case "answer_delta":
			var payload struct {
				Content string `json:"content"`
			}
			_ = json.Unmarshal([]byte(raw), &payload)
			if callbacks.OnAnswer != nil && payload.Content != "" {
				callbacks.OnAnswer(payload.Content)
			}
		case "panel", "panel_action":
			var action PanelAction
			if err := json.Unmarshal([]byte(raw), &action); err == nil && callbacks.OnPanelAction != nil {
				callbacks.OnPanelAction(action)
			}
		case "turn":
			var value Turn
			if err := json.Unmarshal([]byte(raw), &value); err != nil {
				return err
			}
			turn = &value
		}
		event = ""
		dataLines = dataLines[:0]
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := flush(); err != nil {
				return Turn{}, err
			}
			continue
		}
		if strings.HasPrefix(line, "event:") {
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return Turn{}, err
	}
	if err := flush(); err != nil {
		return Turn{}, err
	}
	if turn == nil {
		return Turn{}, fmt.Errorf("AI stream ended without a turn")
	}
	return *turn, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
