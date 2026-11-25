package transcript

import "time"

// The kinds of a TaskRecord. A foreground Agent call is a subagent record.
const (
	TaskKindShell    = "shell"
	TaskKindMonitor  = "monitor"
	TaskKindSubagent = "subagent"
	// TaskKindCall is a foreground shell call.
	TaskKindCall = "call"
)

// The outcomes of a TaskEnd. Empty means finished with no outcome. "stopped" is
// a harness word for killed; the assembler maps it.
const (
	TaskOutcomeSuccess = "success"
	TaskOutcomeFailure = "failure"
	TaskOutcomeKilled  = "killed"
	TaskOutcomeStopped = "stopped"
)

// Where a matched TaskEnd came from.
const (
	TaskEndNotification = "notification"
	TaskEndTaskStop     = "taskstop"
	TaskEndToolResult   = "tool_result"
)

// TaskEnd is a task's end as the transcript or the output file records it.
type TaskEnd struct {
	At      time.Time
	Outcome string
	Source  string
}

// TaskRecord is one task a harness reader parsed, in a harness-neutral shape.
// It holds parse results and the inputs of the clock- and state-based rules
// (turn end, timeout, session state), never their verdicts, so a cached record
// stays valid as time passes.
type TaskRecord struct {
	Kind string
	// ID is a subagent's agent id; empty while no line names it.
	ID string
	// CallID is the tool_use id (Claude) or call_id (Codex) of the subagent
	// call; empty for other kinds and for a launch result with no call line.
	CallID    string
	Label     string
	StartedAt time.Time

	// End is the matched end, nil while none matched.
	End *TaskEnd
	// TurnEnd is when the foreground turn ended; zero while it has not. A
	// foreground call with no result finishes there.
	TurnEnd time.Time
	// Timeout is a Monitor's timeout, as capped when it launched.
	Timeout time.Duration
	// Trailer is a shell's end from its output file's trailer line, with the
	// file's mtime as the time.
	Trailer *TaskEnd

	// TaskID is the id a background launch result named: a shell's
	// backgroundTaskId, a Monitor's taskId or an async subagent's agentId.
	// Empty for a foreground call.
	TaskID string
	// Reported is set once a CLI report line (notification, queue operation,
	// attachment, TaskOutput or TaskStop result) names TaskID. ReportedAt is the
	// latest such line. AwaitingTurnEnd stays true until an assistant turn ends
	// after it: the report wakes an agent that ended its turn to wait.
	Reported        bool
	ReportedAt      time.Time
	AwaitingTurnEnd bool

	// Details by kind; zero values are absent.
	Command string
	// Prompt and SubagentType are an Agent or Task call's input.prompt and
	// input.subagent_type.
	Prompt       string
	SubagentType string
	ExitCode     *int
	// OutputPath is a shell's output file, as the launch result or the final
	// notification names it; withShellOutputs reads a shell's, subagentFilePath
	// a subagent's.
	OutputPath  string
	OutputBytes int64
	LastLine    string
	Model       string
	// UsageFinal is set when a final notification's <usage> filled Tokens and
	// ToolCalls; the subagent's file is not read then.
	UsageFinal  bool
	Tokens      int
	ToolCalls   int
	CurrentCall string
	EventCount  int
	LastEventAt time.Time
}
