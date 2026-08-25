package agent

import (
	"strings"
	"testing"
)

func TestReadSSE(t *testing.T) {
	var answer string
	var action PanelAction
	turn, err := readSSE(strings.NewReader(
		"event: answer_delta\ndata: {\"content\":\"检查系统\"}\n\n"+
			"event: panel_action\ndata: {\"action\":\"open\",\"panel\":{\"type\":\"monitor\",\"title\":\"CPU\"}}\n\n"+
			"event: turn\ndata: {\"type\":\"finished\",\"session\":{\"id\":\"s1\"}}\n\n",
	), Callbacks{
		OnAnswer: func(value string) { answer += value },
		OnPanelAction: func(value PanelAction) {
			action = value
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if answer != "检查系统" || action.Panel.Title != "CPU" || turn.Session.ID != "s1" {
		t.Fatalf("answer=%q action=%#v turn=%#v", answer, action, turn)
	}
}
