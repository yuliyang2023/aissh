package app

import (
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/agent"
	"git.example.com/ai/wrd-ai-cli/internal/trigger"
)

type outputEvent struct{ data []byte }
type inputEvent struct{ data []byte }
type resizeEvent struct{}
type sshExitEvent struct{ err error }
type monitorResultEvent struct {
	panelID string
	content string
	err     error
}
type agentStatusEvent struct{ status string }
type agentThoughtEvent struct{ content string }
type agentAnswerEvent struct{ content string }
type agentTurnEvent struct {
	turn agent.Turn
	err  error
}
type agentPanelEvent struct{ action agent.PanelAction }
type alwaysYesRiskProgressEvent struct {
	pass  int
	total int
}
type alwaysYesRiskEvent struct {
	proposalID  string
	assessments []agent.RiskAssessment
	err         error
}
type approvalEvent struct {
	turn    agent.Turn
	command string
	err     error
}
type rejectionEvent struct {
	turn    agent.Turn
	command string
	err     error
}
type executionResultEvent struct {
	turn agent.Turn
	err  error
}
type delayedErrorTriggerEvent struct {
	match     trigger.ErrorMatch
	command   string
	createdAt time.Time
}

type focusMode uint8

const (
	focusTerminal focusMode = iota
	focusApproval
	focusAI
	focusAIHistory
	focusMonitor
	focusMonitorSlot
	focusPanelMenu
	focusCustomPanel
)

type monitorLayoutMode uint8

const (
	monitorLayoutAuto monitorLayoutMode = iota
	monitorLayoutSingle
	monitorLayoutSplit
	monitorLayout1x3
	monitorLayout2x2
	monitorLayoutGrid
)

type chatMessage struct {
	Role            string
	Content         string
	Time            time.Time
	Proposal        *agent.Proposal
	Historical      bool
	CommandExpanded bool
}

type executionState struct {
	SessionID  string
	ProposalID string
	Command    string
	Output     *contextBuffer
	StartedAt  time.Time
	Inline     bool
	Marker     string
	Pending    []byte
	ExitCode   *int
	Completion string
	WriteJob   *sshWriteJob
	WaitWarned bool
}

type inlineAIState struct {
	answerCache     *inlineAnswerCache
	Prompt          string
	AnchorX         int
	AnchorY         int
	AwaitingPrompt  bool
	Status          string
	StatusPrefix    string
	Response        string
	Error           string
	Done            bool
	Spinner         int
	Started         time.Time
	Proposal        *agent.Proposal
	ApprovalResult  string
	CommandExpanded bool
}
