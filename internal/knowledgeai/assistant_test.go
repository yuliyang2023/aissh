package knowledgeai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"git.example.com/ai/wrd-ai-cli/internal/agent"
	"git.example.com/ai/wrd-ai-cli/internal/catalog"
	"git.example.com/ai/wrd-ai-cli/internal/redact"
)

type scriptedProvider struct {
	turns    []agent.Turn
	requests []agent.Request
	actions  int
	fail     bool
}

func (p *scriptedProvider) Name() string     { return "test" }
func (p *scriptedProvider) Configured() bool { return true }
func (p *scriptedProvider) Stream(_ context.Context, r agent.Request, _ agent.Callbacks) (agent.Turn, error) {
	p.requests = append(p.requests, r)
	if len(p.turns) == 0 {
		if p.fail {
			return agent.Turn{}, errors.New("model unavailable")
		}
		return agent.Turn{Type: "finished", Message: "已保存"}, nil
	}
	t := p.turns[0]
	p.turns = p.turns[1:]
	return t, nil
}
func (p *scriptedProvider) Action(_ context.Context, r agent.Request) (agent.Turn, error) {
	p.actions++
	return agent.Turn{}, nil
}
func proposal(op Operation) agent.Turn {
	b, _ := json.Marshal(op)
	return agent.Turn{Type: "command_proposal", Session: agent.Session{ID: "creation"}, Proposal: &agent.Proposal{ID: "op", Command: "@knowledge " + string(b)}}
}

func TestCreateAndRepeatFactWithoutSSH(t *testing.T) {
	s := catalog.New(t.TempDir(), nil)
	prompt := "192.0.2.10 属于研发部"
	op := Operation{Action: "upsert_record", Title: "资产归属", Key: map[string]string{"IP": "192.0.2.10"}, Values: map[string]string{"部门": "研发部"}, Quote: prompt}
	for _, status := range []string{"created", "unchanged"} {
		a := Assistant{Store: s, Mode: "knowledge", Prompt: prompt, ConversationID: "new-one"}
		p := &scriptedProvider{turns: []agent.Turn{proposal(Operation{Action: "search"}), proposal(op)}}
		_, err := a.Run(context.Background(), p, "", nil)
		if err != nil || len(a.Receipts) != 1 || a.Receipts[0].Status != status || p.actions != 2 {
			t.Fatalf("creation: %+v %v", a.Receipts, err)
		}
		if p.requests[0].Target != nil || p.requests[0].SystemContext["catalog_assistant"] == "" || !strings.Contains(p.requests[2].Output, `"id"`) {
			t.Fatal("missing isolated role or persisted receipt")
		}
	}
	items, _ := s.ListKnowledge()
	if len(items) != 1 || len(items[0].Rows) != 1 || len(items[0].Changes) != 1 {
		t.Fatalf("duplicate: %+v", items)
	}
}

func TestWritesRequireUserFactsAndTheCorrectCreationMode(t *testing.T) {
	op := Operation{Action: "upsert_record", Title: "资产", Key: map[string]string{"IP": "192.0.2.10"}, Values: map[string]string{"部门": "研发部"}, Quote: "192.0.2.10 属于研发部"}
	for _, tc := range []struct {
		prompt, mode string
		alter        func(*Operation)
	}{
		{"192.0.2.10 属于研发部吗？", "knowledge", nil},
		{"例如 192.0.2.10 属于研发部", "knowledge", nil},
		{"不要保存：192.0.2.10 属于研发部", "knowledge", nil},
		{"192.0.2.10 属于研发部", "system", nil},
		{"192.0.2.10 属于研发部", "knowledge", func(o *Operation) { o.Quote = "虚构事实" }},
		{"192.0.2.10 属于研发部", "knowledge", func(o *Operation) { o.Values = map[string]string{"部门": "财务部"} }},
	} {
		s := catalog.New(t.TempDir(), nil)
		current := op
		if tc.alter != nil {
			tc.alter(&current)
		}
		a := Assistant{Store: s, Mode: tc.mode, Prompt: tc.prompt, Filter: redact.New()}
		if _, err := a.execute(current); err == nil {
			t.Fatalf("accepted %q", tc.prompt)
		}
		items, _ := s.ListKnowledge()
		if len(items) != 0 {
			t.Fatal("invalid fact saved")
		}
	}
	for _, command := range []string{"touch /tmp/should-not-run", `@knowledge {"action":"search","shell":"evil"}`, `@knowledge {"action":"search"} {}`} {
		if _, err := Parse(command); err == nil {
			t.Fatalf("accepted command %q", command)
		}
	}
}

func TestPartialSaveSurvivesModelFailureAndFailedTurnsDoNotWrite(t *testing.T) {
	op := Operation{Action: "upsert_record", Title: "归属", Key: map[string]string{"名称": "应用 A"}, Values: map[string]string{"部门": "研发"}, Quote: "应用 A 属于研发"}
	a := Assistant{Store: catalog.New(t.TempDir(), nil), Mode: "knowledge", Prompt: op.Quote}
	p := &scriptedProvider{turns: []agent.Turn{proposal(op)}, fail: true}
	if _, err := a.Run(context.Background(), p, "", nil); err == nil || len(a.Receipts) != 1 {
		t.Fatalf("lost partial success: %+v %v", a.Receipts, err)
	}
	failed := proposal(op)
	failed.Type = "failed"
	failed.Error = "provider failed"
	a.Receipts = nil
	p = &scriptedProvider{turns: []agent.Turn{failed}}
	if _, err := a.Run(context.Background(), p, "", nil); err != nil || p.actions != 0 || len(a.Receipts) != 0 {
		t.Fatal("executed a failed turn")
	}
}

func TestCreationCanUseFactsFromPreviousUserTurns(t *testing.T) {
	a := Assistant{Store: catalog.New(t.TempDir(), nil), Mode: "system", Prompt: "地址是 https://cmdb.example.com", UserFacts: "添加 CMDB，类型 HTTP\n地址是 https://cmdb.example.com", Filter: redact.New()}
	op := Operation{Action: "save_system", Quote: a.Prompt, System: &System{Name: "CMDB", Kind: "http", Endpoint: "https://cmdb.example.com"}}
	if _, err := a.execute(op); err != nil {
		t.Fatal(err)
	}
	d, _ := a.Store.Load()
	if len(d.Resources) != 1 {
		t.Fatal("did not save system")
	}
}
