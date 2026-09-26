package desktop

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/catalog"
	"git.example.com/ai/wrd-ai-cli/internal/credentialui"
)

func exchangeTaskState(t *testing.T, s *Service, session *terminalSession, state credentialui.WorkbenchState) {
	t.Helper()
	data, err := json.Marshal(credentialui.WorkbenchExchange{State: state})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.exchangeWorkbench(session, w, httptest.NewRequest(http.MethodPost, "/workbench", bytes.NewReader(data)))
	if w.Code != http.StatusOK {
		t.Fatalf("exchange: %d %s", w.Code, w.Body.String())
	}
}

func TestPetTasksSurviveCoalescedSnapshotsWithoutDuplicatesOrOutput(t *testing.T) {
	s := New(catalog.New(t.TempDir(), memorySecrets{}))
	one := &terminalSession{id: "one", info: SessionInfo{ID: "one", Name: "生产服务器"}}
	two := &terminalSession{id: "two", info: SessionInfo{ID: "two", Name: "开发服务器"}}
	s.sessions[one.id], s.sessions[two.id] = one, two
	// A fast task may finish before any busy snapshot reaches the desktop.
	state := credentialui.WorkbenchState{
		Messages: []credentialui.WorkbenchMessage{{Content: "private answer"}},
		Proposal: &credentialui.WorkbenchProposal{Command: "private command"},
		Notices: []credentialui.WorkbenchNotice{
			{Sequence: 1, Kind: "approval", Time: time.Now().UTC().Format(time.RFC3339Nano)},
			{Sequence: 2, Kind: "completed", Time: time.Now().UTC().Format(time.RFC3339Nano)},
		},
	}
	exchangeTaskState(t, s, one, state)
	exchangeTaskState(t, s, one, state)
	exchangeTaskState(t, s, two, state)
	state.Busy = true
	exchangeTaskState(t, s, one, state)
	state.Busy, state.Notices = false, nil
	exchangeTaskState(t, s, one, state)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/state", nil)
	r.Header.Set("Authorization", "Bearer secret")
	s.petHandler("secret").ServeHTTP(w, r)
	var pet PetState
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &pet) != nil {
		t.Fatalf("pet snapshot: %s", w.Body.String())
	}
	if len(pet.TaskNotices) != 4 || pet.TaskNotices[1].Kind != "completed" || pet.TaskNotices[1].SessionID != "one" || pet.TaskNotices[3].SessionID != "two" || pet.TaskNotices[3].Sequence != 4 {
		t.Fatalf("lost, repeated or misrouted outcomes: %+v", pet.TaskNotices)
	}
	if len(pet.Incidents) != 0 || len(pet.Notices) != 0 || strings.Contains(w.Body.String(), "private") {
		t.Fatalf("task notice exposed output or became an alert: %s", w.Body.String())
	}
	var event, target string
	focused := false
	s.emit = func(name string, args ...any) { event, target = name, args[0].(string) }
	s.focusMain = func() { focused = true }
	for _, id := range []string{"one", "two", "removed"} {
		r = httptest.NewRequest(http.MethodPost, "/action", strings.NewReader(fmt.Sprintf(`{"kind":"open","id":%q}`, id)))
		r.Header.Set("Authorization", "Bearer secret")
		w = httptest.NewRecorder()
		s.petHandler("secret").ServeHTTP(w, r)
		if id == "removed" {
			if w.Code != http.StatusBadRequest {
				t.Fatal("removed session accepted")
			}
		} else if w.Code != http.StatusNoContent || !focused || event != "monitor-session" || target != id {
			t.Fatal("task bubble did not focus its session")
		}
	}
}

func TestPetTaskMuteDisableFreshnessAndBoundedHistory(t *testing.T) {
	s := New(catalog.New(t.TempDir(), memorySecrets{}))
	session := &terminalSession{id: "one", info: SessionInfo{Name: "服务器"}}
	var sequence uint64
	notice := func(kind string, at time.Time) credentialui.WorkbenchState {
		sequence++
		return credentialui.WorkbenchState{Notices: []credentialui.WorkbenchNotice{{Sequence: sequence, Kind: kind, Time: at.UTC().Format(time.RFC3339Nano)}}}
	}
	for _, mode := range []string{"muted", "disabled"} {
		exchangeTaskState(t, s, session, notice("completed", time.Now()))
		if mode == "muted" {
			if err := s.SnoozeAlerts(15); err != nil {
				t.Fatal(err)
			}
		} else {
			p := s.GetMonitoring().Policy
			p.Enabled = false
			if err := s.SaveAlertPolicy(p); err != nil {
				t.Fatal(err)
			}
		}
		state := notice("completed", time.Now())
		exchangeTaskState(t, s, session, state)
		if len(s.petSnapshot().TaskNotices) != 0 {
			t.Fatalf("%s task was delivered", mode)
		}
		if err := s.SnoozeAlerts(0); err != nil {
			t.Fatal(err)
		}
		p := s.GetMonitoring().Policy
		p.Enabled = true
		if err := s.SaveAlertPolicy(p); err != nil {
			t.Fatal(err)
		}
		exchangeTaskState(t, s, session, state)
		if len(s.petSnapshot().TaskNotices) != 0 {
			t.Fatalf("%s task replayed after resuming", mode)
		}
	}
	exchangeTaskState(t, s, session, notice("completed", time.Now().Add(-time.Minute)))
	exchangeTaskState(t, s, session, notice("completed", time.Now().Add(time.Minute)))
	exchangeTaskState(t, s, session, notice("unknown", time.Now()))
	if len(s.petSnapshot().TaskNotices) != 0 {
		t.Fatal("invalid or stale notice delivered")
	}
	for i := 0; i < 25; i++ {
		exchangeTaskState(t, s, session, notice("completed", time.Now()))
	}
	if len(s.petSnapshot().TaskNotices) != 20 {
		t.Fatal("task notice history unbounded")
	}
}

func TestPetReadSessionIsScopedAndDoesNotConsumeUnseenTasks(t *testing.T) {
	s := New(catalog.New(t.TempDir(), memorySecrets{}))
	one := &terminalSession{id: "one", info: SessionInfo{Name: "同名服务器"}}
	two := &terminalSession{id: "two", info: SessionInfo{Name: "同名服务器"}}
	s.sessions[one.id], s.sessions[two.id] = one, two
	publish := func(session *terminalSession, sequence uint64) string {
		exchangeTaskState(t, s, session, credentialui.WorkbenchState{Notices: []credentialui.WorkbenchNotice{{
			Sequence: sequence, Kind: "completed", Time: time.Now().UTC().Format(time.RFC3339Nano),
		}}})
		return session.workbench.UpdatedAt
	}
	old := publish(one, 1)
	publish(two, 1)
	current := publish(one, 2)
	if err := s.ReadPetSession(one.id, old); err != nil {
		t.Fatal(err)
	}
	if len(s.petSnapshot().Read) != 0 {
		t.Fatal("delayed receipt consumed a newer task")
	}
	if err := s.ReadPetSession(one.id, current); err != nil {
		t.Fatal(err)
	}
	state := s.petSnapshot()
	if len(state.Read) != 1 || state.Read[one.id].Task != 3 || state.Read[two.id].Task != 0 {
		t.Fatalf("read receipt crossed sessions: %+v", state.Read)
	}
	state.Read[one.id] = PetReadSequence{}
	if s.petSnapshot().Read[one.id].Task != 3 {
		t.Fatal("snapshot shares mutable read state")
	}
	publish(one, 3)
	if err := s.ReadPetSession(one.id, current); err != nil {
		t.Fatal(err)
	}
	if s.petSnapshot().Read[one.id].Task != 3 {
		t.Fatal("new task was read without being displayed")
	}
	// The desktop pet can retain an unread entry after the backend's short
	// delivery history has rolled over. Revisiting must still clear that entry.
	for i := uint64(2); i < 25; i++ {
		publish(two, i)
	}
	if err := s.ReadPetSession(one.id, one.workbench.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	state = s.petSnapshot()
	if state.Read[one.id].Task < 4 || state.Read[two.id].Task != 0 {
		t.Fatalf("old unread entry was lost or other session was read: %+v", state.Read)
	}
	if err := s.ReadPetSession("removed", current); err == nil {
		t.Fatal("unknown session accepted")
	}
}

func TestPetReadDoesNotAcknowledgeAlerts(t *testing.T) {
	s := New(catalog.New(t.TempDir(), memorySecrets{}))
	s.alerts.Policy.ConfirmSeconds = 0
	now := time.Now()
	for _, id := range []string{"one", "two"} {
		s.sessions[id] = &terminalSession{id: id, info: SessionInfo{Name: "同名服务器", Status: "running"}, workbench: &credentialui.WorkbenchState{
			UpdatedAt:     now.Format(time.RFC3339Nano),
			AlertMonitors: []credentialui.MonitorPanel{{ID: "alert-nginx", Error: "collector failed", UpdatedAt: now.Format(time.RFC3339Nano)}},
		}}
	}
	s.tickAlerts(now)
	if err := s.ReadPetSession("one", now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	state := s.petSnapshot()
	if len(state.Notices) != 2 || state.Read["one"].Alert != state.Notices[1].Sequence || state.Read["two"].Alert != 0 {
		t.Fatalf("alert reads were not scoped: %+v", state)
	}
	for _, incident := range state.Incidents {
		if incident.Acknowledged || incident.Status != "firing" {
			t.Fatalf("reading changed the underlying incident: %+v", incident)
		}
	}
}
