package schema

import (
	"fmt"
	"slices"
	"strings"
)

// PauseVerb is one way a person recovers a pause.
type PauseVerb string

const (
	VerbAnswer      PauseVerb = "answer"
	VerbResume      PauseVerb = "resume"
	VerbRetry       PauseVerb = "retry"
	VerbStartOver   PauseVerb = "start-over"
	VerbMergeAgain  PauseVerb = "merge-again"
	VerbChooseBase  PauseVerb = "choose-base"
	VerbAllowDeny   PauseVerb = "allow-deny"
	VerbReopenRetry PauseVerb = "reopen-retry"
	VerbStashMerge  PauseVerb = "stash-merge"
)

// PauseReceiver says who gets the reply to an answer-class pause.
type PauseReceiver string

const (
	ReceiverAgent PauseReceiver = "agent"
	ReceiverGx    PauseReceiver = "gx"
)

// VerbInfo is what the web and the TUI show for a verb. Word is how the
// detail's actions and the refusals spell it.
type VerbInfo struct {
	Word    string
	Label   string
	Tooltip string
}

// pauseVerbs gives every verb its word, label and tooltip, so the web and the
// TUI show the same text.
var pauseVerbs = map[PauseVerb]VerbInfo{
	VerbAnswer:      {Word: "answer", Label: "Answer", Tooltip: "Reply to the agent. It goes on from where it stopped."},
	VerbResume:      {Word: "resume", Label: "Resume", Tooltip: "Lift the pause. The ticket waits its turn in the queue."},
	VerbRetry:       {Word: "retry", Label: "Retry", Tooltip: "Start a fresh agent now. Keeps the branch and its commits."},
	VerbStartOver:   {Word: "start-over", Label: "Start over", Tooltip: "Throw away the work and run again from the base. A backup branch is kept."},
	VerbMergeAgain:  {Word: "merge", Label: "Merge again", Tooltip: "Merge the ticket's commits onto its target again."},
	VerbChooseBase:  {Word: "choose-base", Label: "Choose base", Tooltip: "Pick the branch this ticket builds on. The agent goes on from there."},
	VerbAllowDeny:   {Word: "approve", Label: "Allow / Deny", Tooltip: "Answer the agent's permission request. The agent goes on from where it stopped."},
	VerbReopenRetry: {Word: "reopen-and-retry", Label: "Reopen and retry", Tooltip: "Reopen the ticket and start a fresh agent on the stranded commits."},
	VerbStashMerge:  {Word: "stash-merge", Label: "Stash, merge and restore", Tooltip: "Set your changes aside, merge, then put them back. Conflicts are left for you to fix."},
}

// Label is v's button text, or "" for an unknown verb.
func (v PauseVerb) Label() string { return pauseVerbs[v].Label }

// Tooltip is v's help text, or "" for an unknown verb.
func (v PauseVerb) Tooltip() string { return pauseVerbs[v].Tooltip }

// Word is the verb as the detail's actions and the refusals spell it.
func (v PauseVerb) Word() string { return pauseVerbs[v].Word }

// VerbOfWord is the verb that Word spells as w.
func VerbOfWord(w string) (PauseVerb, bool) {
	for v, info := range pauseVerbs {
		if info.Word == w {
			return v, true
		}
	}
	return "", false
}

// Describe is how a refusal names the ticket's status: "paused (repair:
// iteration-error)" for a pause, else the stored status.
func (t Ticket) Describe() string {
	class := t.PausedClass()
	if class == "" {
		return string(t.Status)
	}
	return fmt.Sprintf("paused (%s: %s)", class, t.EffectiveReason())
}

// EffectiveReason is the stored reason, or the class default for a legacy
// status that stores none.
func (t Ticket) EffectiveReason() PauseReason {
	if t.PauseReason == "" {
		return t.PausedClass().DefaultReason()
	}
	return t.PauseReason
}

// RecoveryVerb is the first verb of t's table row, the one a refusal points at,
// or "" when t is not paused or its kind has no row.
func (t Ticket) RecoveryVerb() string {
	if t.PausedClass() == "" {
		return ""
	}
	row, ok := t.EffectiveReason().Row()
	if !ok || len(row.Verbs) == 0 {
		return ""
	}
	return row.Verbs[0].Word()
}

// PauseRow lists the verbs that recover one pause kind, in display order, and
// for an answer-class kind who receives the reply.
type PauseRow struct {
	Verbs    []PauseVerb
	Receiver PauseReceiver
}

func verbs(v ...PauseVerb) PauseRow { return PauseRow{Verbs: v} }

func (r PauseRow) from(who PauseReceiver) PauseRow {
	r.Receiver = who
	return r
}

var (
	retryStartOver = verbs(VerbRetry, VerbStartOver)
	mergeAgain     = verbs(VerbMergeAgain)
)

// pauseRows is the one pause-to-verb table. A kind that only needs a person's
// reply has an answer-class row; a fault has a repair-class one.
var pauseRows = map[PauseReason]PauseRow{
	PauseSelfReported:       verbs(VerbAnswer, VerbResume).from(ReceiverAgent),
	PauseAmbiguousBase:      verbs(VerbChooseBase).from(ReceiverAgent),
	PauseZeroCommit:         verbs(VerbRetry).from(ReceiverAgent),
	PauseBlockedPane:        verbs(VerbAllowDeny, VerbRetry).from(ReceiverAgent),
	PauseBackgroundTaskGate: verbs(VerbRetry).from(ReceiverAgent),
	PauseStrandedCommits:    verbs(VerbReopenRetry).from(ReceiverAgent),
	PauseManual:             verbs(VerbResume, VerbStartOver).from(ReceiverGx),
	PauseDirtyTrunk:         verbs(VerbStashMerge, VerbMergeAgain).from(ReceiverGx),

	PauseMergeUnreported: verbs(VerbRetry, VerbMergeAgain, VerbStartOver),
	PauseMergeConflict:   verbs(VerbMergeAgain, VerbStartOver),
	PauseAmbiguousMerge:  retryStartOver,
	PauseBudgetKilled:    retryStartOver,
	PauseConflictMarkers: retryStartOver,

	PauseIterationError:     retryStartOver,
	PauseSpinning:           retryStartOver,
	PauseRetryExhausted:     retryStartOver,
	PauseHandleMismatch:     retryStartOver,
	PauseDuplicateLive:      retryStartOver,
	PauseAgentNameTaken:     retryStartOver,
	PauseAgentPaneBusy:      retryStartOver,
	PauseAgentPromptStalled: retryStartOver,

	PauseTrunkMissing: mergeAgain,
	PauseMergeError:   mergeAgain,
	PauseNeedsRebase:  mergeAgain,
}

// graphEventsNotPauses are the kinds with no row: they come from the ticket
// graph, and nothing in the web recovers them.
var graphEventsNotPauses = []PauseReason{PauseBlockedCycle, PauseParentDefect}

// Row is r's row in the pause-to-verb table. ok is false for a kind with no
// row.
func (r PauseReason) Row() (PauseRow, bool) {
	row, ok := pauseRows[r]
	return row, ok
}

// PauseKinds returns every pause kind that has a class, in no fixed order.
func PauseKinds() []PauseReason {
	out := make([]PauseReason, 0, len(pauseClasses))
	for r := range pauseClasses {
		out = append(out, r)
	}
	slices.Sort(out)
	return out
}

// checkPauseRows reports each kind in kinds, other than the exempt ones, that
// has no row.
func checkPauseRows(kinds []PauseReason, rows map[PauseReason]PauseRow, exempt []PauseReason) error {
	var problems []string
	for _, k := range kinds {
		if _, ok := rows[k]; !ok && !slices.Contains(exempt, k) {
			problems = append(problems, fmt.Sprintf("%s: no row", k))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("pause-to-verb table: %s", strings.Join(problems, "; "))
	}
	return nil
}
