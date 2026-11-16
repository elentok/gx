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
	ID        string
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

	// Details by kind; zero values are absent.
	Command  string
	ExitCode *int
	// OutputPath is a shell's output file, as the launch result or the final
	// notification names it. It is recorded here and not read yet.
	OutputPath  string
	OutputBytes int64
	LastLine    string
	Model       string
	Tokens      int
	ToolCalls   int
	CurrentCall string
	EventCount  int
	LastEventAt time.Time
}
