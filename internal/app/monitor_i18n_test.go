package app

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode"

	"git.example.com/ai/wrd-ai-cli/internal/agent"
	"git.example.com/ai/wrd-ai-cli/internal/i18n"
)

func TestMonitorAuditLifecycleUsesSelectedLanguage(t *testing.T) {
	for _, language := range []string{"en", "zh-CN"} {
		for _, outcome := range []string{"approved", "rejected", "timeout"} {
			t.Run(language+"/"+outcome, func(t *testing.T) {
				a, terminal := newMonitorTestApp(t)
				a.localizer = i18n.New(language)
				a.config.Language = language
				a.chat, a.monitorDecisions = nil, nil
				p := a.panels.ByID("system")
				p.Title, p.AllowAIRead = "API health", true
				a.scheduleMonitors()
				if a.proposal == nil || terminal.Len() != 0 {
					t.Fatal("collector must wait for review")
				}
				review := a.proposal.Explanation + a.proposal.ExpectedEffect + a.approvalStatus + a.renderApprovalCard()
				wantReview, wantResult := "重复采集", "退出码：0"
				if language == "en" {
					wantReview, wantResult = "every", "Exit code: 0"
				}
				if !strings.Contains(review, wantReview) {
					t.Fatalf("review missing %q: %s", wantReview, review)
				}
				if outcome == "rejected" {
					a.rejectProposal()
					a.scheduleMonitors()
					if a.inBandMonitor != nil || terminal.Len() != 0 {
						t.Fatal("rejected collector executed")
					}
					wantResult = "已拒绝"
					if language == "en" {
						wantResult = "rejected"
					}
				} else {
					a.approveProposal()
					a.scheduleMonitors()
					if a.inBandMonitor == nil {
						t.Fatal("approved collector did not start")
					}
					if outcome == "timeout" {
						a.checkInBandMonitorTimeout(time.Now().Add(time.Hour))
						wantResult = "退出状态未知"
						if language == "en" {
							wantResult = "Exit status unknown"
						}
					} else {
						marker := a.inBandMonitor.marker
						a.processSSHOutput([]byte(marker + "BEGIN__\r\nactive\r\n" + marker + "END__:0__\r\nroot@server:~# "))
					}
				}
				messages, err := json.Marshal(a.chat)
				if err != nil {
					t.Fatal(err)
				}
				result := monitorScreenText(a) + p.LastError + string(messages)
				if !strings.Contains(result, wantResult) {
					t.Fatalf("result missing %q: %s", wantResult, result)
				}
				if language == "en" {
					assertNoChineseUI(t, review+result)
				}
			})
		}
	}
}

func TestEnglishPanelLifecycleAndSourceReview(t *testing.T) {
	a, err := New(Config{SSHArgs: []string{"server"}, Language: "en"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.cancel)
	action := agent.PanelAction{Action: "create", Panel: agent.PanelSpec{ID: "api-health", Title: "API health", Type: "monitor", Command: "systemctl is-active demo-api", IntervalMS: 3000}}
	for _, operation := range []string{"create", "update", "append", "close", "unknown"} {
		action.Action = operation
		_, message, err := a.executePanelAction(action)
		if err != nil {
			message += err.Error()
		}
		if message == "" {
			t.Fatalf("%s returned no status", operation)
		}
		assertNoChineseUI(t, message)
	}
	a.ensureMonitorAudit(monitorAudit{PanelID: "api", SourceID: "health", Command: "printf active", Interval: 3 * time.Second, PipelineID: "probe"})
	if a.proposal == nil || !strings.Contains(a.proposal.Explanation, "Source: health") || !strings.Contains(a.proposal.Explanation, "AI analysis") {
		t.Fatal("source review omitted collection and AI access disclosures")
	}
	assertNoChineseUI(t, a.proposal.Explanation+a.proposal.ExpectedEffect+a.approvalStatus)
}

func assertNoChineseUI(t *testing.T, text string) {
	t.Helper()
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			t.Fatalf("English UI contains Chinese text: %s", text)
		}
	}
}
