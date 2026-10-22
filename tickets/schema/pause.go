package schema

import (
	"regexp"
	"strings"
)

// PauseReason says why a ticket is Paused. It is the stored `pause_reason`
// frontmatter field. The empty string is valid: it means the ticket is not
// paused.
type PauseReason string

const (
	PauseSelfReported       PauseReason = "self-reported"
	PauseAmbiguousBase      PauseReason = "ambiguous-base"
	PauseZeroCommit         PauseReason = "zero-commit"
	PauseMergeUnreported    PauseReason = "merge-unreported"
	PauseBlockedPane        PauseReason = "blocked-pane"
	PauseSpinning           PauseReason = "spinning"
	PauseRetryExhausted     PauseReason = "retry-exhausted"
	PauseHandleMismatch     PauseReason = "handle-mismatch"
	PauseDuplicateLive      PauseReason = "duplicate-live"
	PauseAgentNameTaken     PauseReason = "agent_name_taken"
	PauseAgentPaneBusy      PauseReason = "agent_pane_busy"
	PauseAgentPromptStalled PauseReason = "agent_prompt_stalled"
	PauseBackgroundTaskGate PauseReason = "background-task-gate"
	PauseMergeConflict      PauseReason = "merge-conflict"
	PauseAmbiguousMerge     PauseReason = "ambiguous-merge"
	PauseDirtyTrunk         PauseReason = "dirty-trunk"
	PauseTrunkMissing       PauseReason = "trunk-missing"
	PauseStrandedCommits    PauseReason = "stranded-commits"
	PauseMergeError         PauseReason = "merge-error"
	PauseBlockedCycle       PauseReason = "blocked-cycle"
	PauseParentDefect       PauseReason = "parent-defect"
	PauseBudgetKilled       PauseReason = "budget-killed"
	PauseIterationError     PauseReason = "iteration-error"
	PauseManual             PauseReason = "manual"
)

// PauseClass says what a person does about a pause: answer a question, or
// repair a fault. Every rule that used to key on the stored needs-answer /
// needs-repair status keys on the class instead.
type PauseClass string

const (
	PauseClassAnswer PauseClass = "answer"
	PauseClassRepair PauseClass = "repair"
)

// pauseClasses is the one table that gives every reason its class.
var pauseClasses = map[PauseReason]PauseClass{
	PauseSelfReported:       PauseClassAnswer,
	PauseAmbiguousBase:      PauseClassAnswer,
	PauseZeroCommit:         PauseClassAnswer,
	PauseBlockedPane:        PauseClassAnswer,
	PauseBackgroundTaskGate: PauseClassAnswer,
	PauseStrandedCommits:    PauseClassAnswer,

	PauseMergeUnreported:    PauseClassRepair,
	PauseSpinning:           PauseClassRepair,
	PauseRetryExhausted:     PauseClassRepair,
	PauseHandleMismatch:     PauseClassRepair,
	PauseDuplicateLive:      PauseClassRepair,
	PauseAgentNameTaken:     PauseClassRepair,
	PauseAgentPaneBusy:      PauseClassRepair,
	PauseAgentPromptStalled: PauseClassRepair,
	PauseMergeConflict:      PauseClassRepair,
	PauseAmbiguousMerge:     PauseClassRepair,
	PauseDirtyTrunk:         PauseClassRepair,
	PauseTrunkMissing:       PauseClassRepair,
	PauseMergeError:         PauseClassRepair,
	PauseBlockedCycle:       PauseClassRepair,
	PauseParentDefect:       PauseClassRepair,
	PauseBudgetKilled:       PauseClassRepair,
	PauseIterationError:     PauseClassRepair,
	PauseManual:             PauseClassRepair,
}

// pauseLabels is the one table of plain-words labels the TUI and the web show
// for each reason, grouped as the spec groups them.
var pauseLabels = map[PauseReason]string{
	// Waiting for a person.
	PauseSelfReported:  "Waiting for your answer",
	PauseAmbiguousBase: "Ambiguous base",
	// Agent stopped or stuck.
	PauseZeroCommit:         "Agent stopped without a result",
	PauseMergeUnreported:    "Merge agent stopped without a result",
	PauseBlockedPane:        "Agent stuck",
	PauseSpinning:           "Agent looping",
	PauseRetryExhausted:     "Retries used up",
	PauseHandleMismatch:     "Session mismatch",
	PauseDuplicateLive:      "Already running",
	PauseAgentNameTaken:     "Could not start the agent",
	PauseAgentPaneBusy:      "Could not start the agent",
	PauseAgentPromptStalled: "Could not start the agent",
	// Background task.
	PauseBackgroundTaskGate: "Background task still running",
	// Merging.
	PauseMergeConflict:   "Merge conflict",
	PauseAmbiguousMerge:  "Unclear what to merge",
	PauseDirtyTrunk:      "Trunk has local changes",
	PauseTrunkMissing:    "Trunk not found",
	PauseStrandedCommits: "Stranded commits",
	PauseMergeError:      "Merge failed",
	// Ticket graph.
	PauseBlockedCycle: "Dependency loop",
	PauseParentDefect: "Parent ticket missing or wrong",
	// Limits and other.
	PauseBudgetKilled:   "Daily budget reached",
	PauseIterationError: "Run failed",
	PauseManual:         "Paused by you",
}

// Label is r's plain-words label, or "" for an empty or unknown reason.
func (r PauseReason) Label() string { return pauseLabels[r] }

// Valid reports whether r is a known reason or the empty "absent" value.
func (r PauseReason) Valid() bool {
	if r == "" {
		return true
	}
	_, ok := pauseClasses[r]
	return ok
}

// Class is r's pause class, or "" for an unknown or empty reason.
func (r PauseReason) Class() PauseClass { return pauseClasses[r] }

// Default reasons for a legacy paused ticket that recorded none.
const (
	defaultAnswerReason = PauseZeroCommit
	defaultRepairReason = PauseIterationError
)

// DefaultReason is the reason a legacy ticket of this class gets when it
// recorded none.
func (c PauseClass) DefaultReason() PauseReason {
	if c == PauseClassAnswer {
		return defaultAnswerReason
	}
	return defaultRepairReason
}

// legacyReasons maps the retired reason spellings (an old `park_kind`) to
// their new ones. Part of the read shim.
var legacyReasons = map[string]PauseReason{
	"manual-park":    PauseManual,
	"ambiguous-land": PauseAmbiguousMerge,
	"land-conflict":  PauseMergeConflict,
	"land-error":     PauseMergeError,
}

// ParsePauseReason reads a stored or legacy reason spelling.
func ParsePauseReason(s string) PauseReason {
	if r, ok := legacyReasons[s]; ok {
		return r
	}
	return PauseReason(s)
}

// LegacyStatus is the stored status this class used before `paused`.
// Wire formats and notification wording still use it as their label.
func (c PauseClass) LegacyStatus() Status {
	if c == PauseClassAnswer {
		return StatusLegacyAnswer
	}
	return StatusLegacyRepair
}

// LegacyKind is r spelled as the old event kind (the old `park_kind`). The
// spellings differ from the current reasons for four reasons.
func (r PauseReason) LegacyKind() string {
	for old, nu := range legacyReasons {
		if nu == r {
			return old
		}
	}
	return string(r)
}

// PausedReason returns the reason to store for a pause of class that was
// raised with the (possibly legacy) kind spelling. A kind that is empty, unknown
// or of the other class falls back to the class default.
func PausedReason(kind string, class PauseClass) PauseReason {
	if r := ParsePauseReason(kind); r.Class() == class {
		return r
	}
	return class.DefaultReason()
}

// PausedClass is the class of t when it is paused (or still carries an old
// needs-answer / needs-repair status), else "".
func (t Ticket) PausedClass() PauseClass {
	switch t.Status {
	case StatusLegacyAnswer:
		return PauseClassAnswer
	case StatusLegacyRepair:
		return PauseClassRepair
	case StatusPaused:
		return t.PauseReason.Class()
	}
	return ""
}

// Legacy body headings, accepted by the read shim only.
var legacyPauseHeadings = []string{"## Needs Answer", "## Needs Repair"}

// PausedHeading is the body heading under which a pause explains itself.
const PausedHeading = "## Paused"

// ShimLegacyPause returns t with an old needs-answer / needs-repair status
// read as paused, in memory. The reason comes from the old park_kind; with
// none (or an unknown or class-mismatched one) the status's default applies.
// Any other ticket is returned unchanged.
func ShimLegacyPause(t Ticket) Ticket {
	var class PauseClass
	switch t.Status {
	case StatusLegacyAnswer:
		class = PauseClassAnswer
	case StatusLegacyRepair:
		class = PauseClassRepair
	default:
		return t
	}
	t.Status = StatusPaused
	if t.PauseReason == "" {
		t.PauseReason = ParsePauseReason(string(t.PauseKind))
	}
	if t.PauseReason.Class() != class {
		t.PauseReason = class.DefaultReason()
	}
	t.PauseKind = ""
	return t
}

var markdownMarkerRe = regexp.MustCompile(`[*_` + "`" + `]+`)

// PauseReasonText is the first non-empty line under body's `## Paused`
// heading (or an old heading), markdown markers stripped, or "". It is
// derived on every read and never stored.
func PauseReasonText(body string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if line != PausedHeading && line != legacyPauseHeadings[0] && line != legacyPauseHeadings[1] {
			continue
		}
		for _, next := range lines[i+1:] {
			if trimmed := strings.TrimSpace(next); trimmed != "" {
				return strings.TrimSpace(markdownMarkerRe.ReplaceAllString(trimmed, ""))
			}
		}
		return ""
	}
	return ""
}
