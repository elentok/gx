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

// VerbInfo is what the web and the TUI show for a verb. Web marks a verb the
// web serves; the table check fails a row that has none.
type VerbInfo struct {
	Label   string
	Tooltip string
	Web     bool
}

// pauseVerbs gives every verb its label and tooltip, so the web and the TUI
// show the same text.
var pauseVerbs = map[PauseVerb]VerbInfo{
	VerbAnswer:      {"Answer", "Reply to the agent. It goes on from where it stopped.", true},
	VerbResume:      {"Resume", "Lift the pause. The ticket waits its turn in the queue.", true},
	VerbRetry:       {"Retry", "Start a fresh agent now. Keeps the branch and its commits.", true},
	VerbStartOver:   {"Start over", "Throw away the work and run again from the base. A backup branch is kept.", true},
	VerbMergeAgain:  {"Merge again", "Merge the ticket's commits onto its target again.", true},
	VerbChooseBase:  {"Choose base", "Pick the branch this ticket builds on. The agent goes on from there.", true},
	VerbAllowDeny:   {"Allow / Deny", "Answer the agent's permission request. The agent goes on from where it stopped.", true},
	VerbReopenRetry: {"Reopen and retry", "Reopen the ticket and start a fresh agent on the stranded commits.", true},
	VerbStashMerge:  {"Stash, merge and restore", "Set your changes aside, merge, then put them back. Conflicts are left for you to fix.", true},
}

// Label is v's button text, or "" for an unknown verb.
func (v PauseVerb) Label() string { return pauseVerbs[v].Label }

// Tooltip is v's help text, or "" for an unknown verb.
func (v PauseVerb) Tooltip() string { return pauseVerbs[v].Tooltip }

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
// has no row or whose row lists no verb the web serves.
func checkPauseRows(kinds []PauseReason, rows map[PauseReason]PauseRow, verbInfo map[PauseVerb]VerbInfo, exempt []PauseReason) error {
	var problems []string
	for _, k := range kinds {
		if slices.Contains(exempt, k) {
			continue
		}
		row, ok := rows[k]
		if !ok {
			problems = append(problems, fmt.Sprintf("%s: no row", k))
			continue
		}
		if !slices.ContainsFunc(row.Verbs, func(v PauseVerb) bool { return verbInfo[v].Web }) {
			problems = append(problems, fmt.Sprintf("%s: no verb the web serves", k))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("pause-to-verb table: %s", strings.Join(problems, "; "))
	}
	return nil
}
