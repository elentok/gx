package recovery

import (
	"errors"
	"fmt"
	"slices"

	"github.com/elentok/gx/events"
)

// Provenance the server stamps on every verb a rule remedy calls.
const (
	ActorRecovery = "recovery"
	ViaServer     = "server"
)

// Failure is one event that may start recovery.
type Failure struct {
	Address string
	Type    events.Type
	Kind    events.Kind
	Reason  string
	// Parent is the ID-derived parent a ticket-graph defect backfills.
	Parent string
}

// Triggers reports whether a failure of this shape may start recovery at all:
// every needs-repair kind except budget-killed (a deliberate stop), needs-answer
// zero-commit, needs-answer blocked-pane, a deadlock, a held background-task
// gate (R10) and a ticket-graph defect a scan found (R14). Self-reported and
// manual parks and commitless finishes are a person's or the ticket's own
// decision, never recovered; a rate-limit pause (R8) is healthy waiting. A
// blocked pane still needs a match (NeedsMatch).
func (f Failure) Triggers() bool {
	switch f.Type {
	case events.BackgroundTaskGateHeld, events.TicketGraphDefect:
		return true
	case events.NeedsRepair:
		return f.Kind != events.BudgetKilled && f.Kind != events.SelfReported && f.Kind != events.ManualPark
	case events.NeedsAnswer:
		return f.Kind == events.ZeroCommit || f.Kind == events.BlockedPane
	case events.Deadlocked:
		return true
	}
	return false
}

// NeedsMatch is true for failures recovered only when a catalog entry matches:
// a blocked pane is recovered only on an allow-listed dialog, so an unmatched
// one stays a person's to answer instead of becoming an investigation.
func (f Failure) NeedsMatch() bool { return f.Kind == events.BlockedPane }

// DiagnosisOnly is true for failures with no ticket to act on: recovery may
// diagnose them but no rule remedy runs.
func (f Failure) DiagnosisOnly() bool { return f.Type == events.Deadlocked }

// Result is what a server verb answered a remedy with.
type Result struct {
	Actor   string
	Via     string
	Refused bool
	Reason  string
	Message string
}

// Verbs are the server's own verbs as a remedy sees them: the same refusals,
// land lock and events as a person's call, stamped actor recovery.
type Verbs interface {
	// Do makes one verb call (see the Verb constants).
	Do(c Call) (Result, error)
	// LaunchPrompt is the prompt a fresh iteration of the ticket is launched
	// with. It is a read, not a verb: the run log never records the prompt.
	LaunchPrompt(address string) (string, error)
}

// The verbs a rule remedy may call.
const (
	VerbPark           = "park"
	VerbRelaunch       = "relaunch"
	VerbCommitlessDone = "commitless-done"
	VerbNudge          = "nudge"
	// VerbClosePane closes the iteration's tab; no live pane is not a refusal.
	VerbClosePane = "close-pane"
	// VerbWait waits once, longer than the loop's own compaction wait, for the
	// iteration's pane to settle; no live pane is a refusal.
	VerbWait = "wait"
	// VerbReleaseGate force-releases the iteration's held background-task gate
	// after fresh checks: the pane is idle, the worktree clean and the branch
	// has commits ahead. A failed check is a refusal.
	VerbReleaseGate = "release-gate"
	// VerbFinish waits, bounded, for the released iteration's ordinary finish
	// path to end; a run still live at the bound is a refusal.
	VerbFinish = "finish"
	// VerbSetParent writes the ticket's parent, refusing when the ticket's
	// parent is no longer a defect or the named parent is not in the epic.
	VerbSetParent = "set-parent"
)

// ReasonVerbNotGranted is the refusal of a verb outside the entry's Verbs.
const ReasonVerbNotGranted = "verb-not-granted"

// Run makes the calls in order, stopping at the first error or refusal.
func Run(v Verbs, calls ...Call) error {
	for _, c := range calls {
		res, err := v.Do(c)
		if err == nil && res.Refused {
			err = fmt.Errorf("%s refused: %s", c.Verb, res.Reason)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// Remedy is a rule entry's fix, written in Go against the server's verbs.
type Remedy func(f Failure, v Verbs) error

// Apply runs the entry's remedy with only the entry's Verbs granted. A remedy
// that swallows a not-granted refusal still fails, so the outcome records it.
func (e Entry) Apply(f Failure, v Verbs) error {
	g := &granted{verbs: e.Verbs, v: v}
	if err := e.Remedy(f, g); err != nil {
		return err
	}
	return g.denied
}

// granted refuses every verb outside its list before it reaches v.
type granted struct {
	verbs  []string
	v      Verbs
	denied error
}

func (g *granted) Do(c Call) (Result, error) {
	if !slices.Contains(g.verbs, c.Verb) {
		res := Result{Refused: true, Reason: ReasonVerbNotGranted, Message: c.Verb + " is not one of the entry's verbs"}
		g.denied = errors.Join(g.denied, fmt.Errorf("%s refused: %s", c.Verb, res.Reason))
		return res, nil
	}
	return g.v.Do(c)
}

func (g *granted) LaunchPrompt(address string) (string, error) { return g.v.LaunchPrompt(address) }

// Runnable reports whether the matched entry may run unattended as a rule: an
// agent or high-authority entry needs a person or a pane, so it does not.
// A blocked pane runs only on a rule entry, which carries the dialog allow-list
// in its predicate.
func (e Entry) Runnable(f Failure) bool {
	if e.Executor != ExecutorRule || e.Authority == AuthorityHigh || e.Remedy == nil {
		return false
	}
	return !f.DiagnosisOnly()
}
