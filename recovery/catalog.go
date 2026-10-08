// Package recovery owns the recovery catalog: failure signatures, each mapped
// to who handles it (a rule or an agent), how much authority it has, and which
// remedy verbs it may use. The catalog is Go data and the matcher is pure.
package recovery

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/transcript"
)

type Executor string

const (
	ExecutorRule  Executor = "rule"
	ExecutorAgent Executor = "agent"
	// ExecutorPerson entries only escalate: neither a rule nor an agent may act.
	ExecutorPerson Executor = "person"
)

type Authority string

const (
	AuthorityLow    Authority = "low"
	AuthorityMedium Authority = "medium"
	AuthorityHigh   Authority = "high"
)

// Event is the slice of a run-log event the matcher reads.
type Event struct {
	Type events.Type
	Kind events.Kind
	// Text is the iteration's last assistant text, set on the failure event
	// only when the matcher's caller read the transcript.
	Text string
	// Reason is the event's reason as the run log recorded it.
	Reason string
	Time   time.Time
}

// Entry is one catalogued failure. It matches when the newest event in the
// sequence has Type and Kind and Predicate (if any) accepts the whole sequence.
type Entry struct {
	ID        string                 `json:"id"`
	Type      events.Type            `json:"type"`
	Kind      events.Kind            `json:"kind"`
	Predicate func(seq []Event) bool `json:"-"`
	Executor  Executor               `json:"executor"`
	Authority Authority              `json:"authority"`
	// Verbs are the remedy verbs the entry may apply.
	Verbs   []string `json:"verbs"`
	Enabled bool     `json:"enabled"`
	// Remedy is the Go fix of a rule entry; nil for agent entries.
	Remedy Remedy `json:"-"`
}

// Catalog is the set of entries plus the global kill switch.
type Catalog struct {
	Enabled bool    `json:"enabled"`
	Entries []Entry `json:"entries"`
}

// NotCatalogued records failures deliberately without an entry.
var NotCatalogued = map[string]string{
	"R13": "vanishes under the server's durable queue",
	// A catalog entry would re-send on top of the chat sink's own resend and
	// amplify R1's notification storm.
	"R9": "the chat sink already re-sends a transient failure: one backed-off retry, then a requeue to the next flush; a 4xx is never retried",
}

// Default is the shipped catalog: kill switch on, one entry per R-ticket as
// they land.
func Default() Catalog {
	// R12 is a narrower zero-commit park than R2's and R3's, so it goes first.
	entries := []Entry{r12StalledReclaim(), r1Spin(), r2UnexecutedToolCall(), r5PromptNeverDelivered(), r7CompactionTimedOut()}
	entries = append(entries, r3LandRecoverable()...)
	entries = append(entries, r4BlockedPaneDialog())
	// R11 is a narrower name-taken park than R6's, so it goes first.
	entries = append(entries, r11ClaimClobbered())
	entries = append(entries, r6LaunchCollision()...)
	return Catalog{Enabled: true, Entries: append(entries, r8RateLimitPause(), r10BackgroundGateHeld(), r14ParentDefect())}
}

// ParentDefect is R14's signature, checked per ticket file with no log: a
// lettered ticket whose parent is missing or is not an ancestor its ID
// allows. want is the ID-derived parent to backfill.
func ParentDefect(id string, parent *string) (want string, defect bool) {
	want, lettered := tickets.IDParent(id)
	if !lettered || (parent != nil && tickets.ParentAllowed(id, *parent)) {
		return "", false
	}
	return want, true
}

// r14ParentDefect is a ticket-graph defect a scan of the epic's issue files
// found (ParentDefect), so the event alone is the signature. The ID encodes the
// intended parent, so a rule backfills it with a direct write; medium authority
// because a wrong parent changes scheduling scope and Queue-tab nesting, though
// visibly rather than as a deadlock. Launches disabled: both live cases were
// fixed in code, so there is no S0 event data behind it.
func r14ParentDefect() Entry {
	return Entry{
		ID: "R14", Type: events.TicketGraphDefect, Kind: events.ParentDefect,
		Executor: ExecutorRule, Authority: AuthorityMedium, Verbs: []string{"set-parent"},
		Remedy: func(f Failure, v Verbs) error {
			res, err := v.SetParent(f.Address, f.Reason)
			if err == nil && res.Refused {
				err = fmt.Errorf("refused: %s", res.Reason)
			}
			if err != nil {
				return fmt.Errorf("backfilling parent %s: %w", f.Reason, err)
			}
			return nil
		},
	}
}

// r10BackgroundGateHeld is a finish gated on a background task nothing ever
// reported ended, so the gate never releases. The run log cannot tell that
// from a task still running: the remedy, not a predicate, must re-check the
// pane is idle (a fresh read, never a cached status), the worktree is clean
// and the branch has commits ahead before it force-releases the gate and runs
// the ordinary finish path. It never parks: the commit exists. Medium
// authority because releasing early finishes an agent that is still working.
// Launches disabled: the S0 holds predate the fix of their cause.
func r10BackgroundGateHeld() Entry {
	return Entry{
		ID: "R10", Type: events.BackgroundTaskGateHeld, Kind: events.BackgroundTaskGate,
		Executor: ExecutorRule, Authority: AuthorityMedium, Verbs: []string{"release-gate", "finish"},
		Remedy: func(f Failure, v Verbs) error {
			res, err := v.ReleaseGate(f.Address)
			if err == nil && !res.Refused {
				res, err = v.Finish(f.Address)
			}
			if err == nil && res.Refused {
				err = fmt.Errorf("refused: %s", res.Reason)
			}
			if err != nil {
				return fmt.Errorf("releasing the background-task gate: %w", err)
			}
			return nil
		},
	}
}

// r11ClaimClobbered is a second launch of a ticket whose own iteration is still
// live: an agent's full-file write reverted its claimed status to open, so the
// scheduler launched it again and herdr refused the taken name. The run log
// shows it as a name-taken park whose launch-failed follows an
// iteration-started that nothing closed. The fix restores claimed with the live
// iteration's session and keeps the duplicate from relaunching; the live tab is
// never closed, it holds the real work. An agent acts because no server verb
// restores a claim, and a false positive re-claims a ticket a person reset on
// purpose. Launches disabled: the per-ticket lock and server-owned status
// writes should make it unreachable, and there is no S0 event data behind it.
func r11ClaimClobbered() Entry {
	return Entry{
		ID: "R11", Type: events.NeedsRepair, Kind: events.AgentNameTaken,
		Predicate: func(seq []Event) bool {
			failed := false
			for _, e := range slices.Backward(seq[:len(seq)-1]) {
				switch e.Type {
				case events.LaunchFailed:
					failed = failed || e.Kind == events.AgentNameTaken
				case events.IterationStarted:
					if failed {
						return true
					}
				case events.IterationFinished, events.NeedsRepair, events.NeedsAnswer, events.Commitless,
					events.CherryPicked, events.ManualLand, events.TicketReset:
					if failed {
						return false
					}
				}
			}
			return false
		},
		Executor: ExecutorAgent, Authority: AuthorityMedium, Verbs: []string{"reclaim"},
	}
}

// reclaimStallWindow is how soon after a reclaim a zero-commit finish means the
// pane was idle because it never started, not because it finished.
const reclaimStallWindow = 2 * time.Second

// r12StalledReclaim is a reclaim onto a pane that stalled earlier: the idle
// pane reads as finished, so the reclaimed iteration parks zero-commit almost
// at once while the stalled agent sits untouched. The fix closes that stale tab
// and relaunches fresh (R5's path). An agent acts because the run log carries
// neither the pane's state_change_seq nor the stalled session, and it must see
// both unchanged before closing: a tab whose agent was idle between turns
// loses its context. Launches disabled: reclaimed is only streamed, never
// written to the run log, so there is no S0 event data and no real log can
// match it yet.
func r12StalledReclaim() Entry {
	return Entry{
		ID: "R12", Type: events.NeedsAnswer, Kind: events.ZeroCommit,
		Predicate: func(seq []Event) bool {
			park := seq[len(seq)-1]
			reclaimed := false
			for _, e := range slices.Backward(seq[:len(seq)-1]) {
				switch {
				case e.Type == events.CherryPicked || e.Type == events.ManualLand || e.Type == events.TicketReset:
					return false
				case reclaimed:
					if e.Kind == events.AgentPromptStalled {
						return true
					}
				case e.Type == events.Reclaimed:
					if e.Time.IsZero() || park.Time.IsZero() || park.Time.Sub(e.Time) >= reclaimStallWindow {
						return false
					}
					reclaimed = true
				case e.Type == events.IterationStarted || e.Type == events.NeedsAnswer || e.Type == events.NeedsRepair || e.Type == events.Commitless:
					return false
				}
			}
			return false
		},
		Executor: ExecutorAgent, Authority: AuthorityMedium, Verbs: []string{"close-pane", "relaunch"},
	}
}

// rateLimitResetsRE is a pause gx waits out itself: Claude's rate limit (its
// reset time is optional) or a Codex quota with a known reset. A Codex quota
// with no reset is a policy stop, not waiting, so it stays uncatalogued.
var rateLimitResetsRE = regexp.MustCompile(`^(rate limit detected|Codex \S+ quota exhausted, resets )`)

// r8RateLimitPause is healthy waiting, not a failure: gx already blocks the
// iteration until the reset and logs resumed. It is catalogued only so recovery
// recognizes a deliberately idle pane and never nudges it, which is also why a
// pause never triggers recovery and the remedy does nothing. Launches enabled:
// S0 emits the pause it matches.
func r8RateLimitPause() Entry {
	return Entry{
		ID: "R8", Type: events.PausedRateLimit,
		Predicate: func(seq []Event) bool { return rateLimitResetsRE.MatchString(seq[len(seq)-1].Reason) },
		Executor:  ExecutorRule, Authority: AuthorityLow, Enabled: true,
		Remedy: func(Failure, Verbs) error { return nil },
	}
}

// r6LaunchCollision is a ticket parked because herdr refused its launch. A busy
// pane is transient, so a rule relaunches once. A taken name needs herdr's
// candidate block read: a leaked pane in this ticket's own worktree is closed
// and the ticket relaunched, while another ticket's live agent means this
// second launch is abandoned and the park stands. Clearing the ticket back to
// open is high authority, so it is outside the grant and the agent may only
// propose it. Both require a launch-failed event of the same kind, so a park
// that merely reuses the kind is not a collision. Launches disabled: there is
// no S0 launch-failed event data behind it yet.
func r6LaunchCollision() []Entry {
	busy := Entry{
		ID: "R6", Type: events.NeedsRepair, Kind: events.AgentPaneBusy,
		Predicate: afterLaunchFailed(events.AgentPaneBusy),
		Executor:  ExecutorRule, Authority: AuthorityLow, Verbs: []string{"relaunch"},
		Remedy: func(f Failure, v Verbs) error {
			_, err := v.Relaunch(f.Address)
			return err
		},
	}
	taken := Entry{
		ID: "R6", Type: events.NeedsRepair, Kind: events.AgentNameTaken,
		Predicate: afterLaunchFailed(events.AgentNameTaken),
		Executor:  ExecutorAgent, Authority: AuthorityMedium, Verbs: []string{"close-pane", "relaunch"},
	}
	return []Entry{busy, taken}
}

func afterLaunchFailed(kind events.Kind) func(seq []Event) bool {
	return func(seq []Event) bool {
		return slices.ContainsFunc(seq[:len(seq)-1], func(ev Event) bool {
			return ev.Type == events.LaunchFailed && ev.Kind == kind
		})
	}
}

// r4BlockedPaneDialog is a pane parked on a prompt gx did not send. The run
// log carries no pane text, so the matcher cannot tell a known dialog from an
// operator's own: an agent reads the pane and answers only an allow-listed,
// content-free dialog (trust-this-folder, a benign "continue?"), escalating
// everything else. A wrong answer is irreversible, which is why the allow-list
// lives with the agent and not in a predicate guessing from the reason.
// Launches disabled: the few S0 events behind it never showed which dialog
// the pane held, so nothing yet justifies the allow-list.
func r4BlockedPaneDialog() Entry {
	return Entry{
		ID: "R4", Type: events.NeedsAnswer, Kind: events.BlockedPane,
		Executor: ExecutorAgent, Authority: AuthorityMedium, Verbs: []string{"answer"},
	}
}

var (
	claimsWorkPresentRE = regexp.MustCompile(`(?i)\balready (present|implemented|merged|landed|exists?|done|on the (feature )?branch)\b`)
	landedElsewhereRE   = regexp.MustCompile(`(?i)\b(in|by|via|with|inside|as part of) (another|an? sibling|the sibling|sibling|an upstream|the upstream)\b`)
	// lostCommitsRE is the reason of an ambiguous land whose iteration branch
	// is gone, from reconcile and from the server's crashed-land verify.
	lostCommitsRE = regexp.MustCompile(`no longer exists to recover them|cannot tell whether it landed: unrecoverable$`)
)

// r3LandRecoverable is work that is on the feature branch (or a recoverable
// branch) but gx cannot attribute: a done ticket whose commits are missing, or
// a zero-commit finish whose last turn claims the work is already present.
// One catalogued failure with several signatures, so entries sharing the ID
// (a disable by ID covers all), most specific first. An agent proves presence
// with verify and the acceptance criteria before it lands a recoverable
// branch. Launches disabled: there is no S0 event data showing it occurs yet.
//
// Two branches never act unattended. Work that landed inside another ticket's
// commits has no commit to attribute, so commitless-done is the only fix, and
// a false one silently drops the ticket from the epic: high authority, so it
// is proposed. Lost commits with no iteration branch only escalate: finding
// their range is out of scope, and recovery must never invent one.
func r3LandRecoverable() []Entry {
	base := Entry{
		ID: "R3", Executor: ExecutorAgent, Authority: AuthorityMedium,
		Verbs: []string{"verify", "land"},
	}
	lost := Entry{
		ID: "R3", Type: events.NeedsRepair, Kind: events.AmbiguousLand,
		Predicate: func(seq []Event) bool { return lostCommitsRE.MatchString(seq[len(seq)-1].Reason) },
		Executor:  ExecutorPerson, Authority: AuthorityHigh,
	}
	unrecoverable := base
	unrecoverable.Type, unrecoverable.Kind = events.NeedsRepair, events.AmbiguousLand
	elsewhere := Entry{
		ID: "R3", Type: events.NeedsAnswer, Kind: events.ZeroCommit,
		Predicate: func(seq []Event) bool {
			text := seq[len(seq)-1].Text
			return claimsWorkPresentRE.MatchString(text) && landedElsewhereRE.MatchString(text)
		},
		Executor: ExecutorRule, Authority: AuthorityHigh, Verbs: []string{"commitless-done"},
		Remedy: func(f Failure, v Verbs) error {
			_, err := v.CommitlessDone(f.Address)
			return err
		},
	}
	claimed := base
	claimed.Type, claimed.Kind = events.NeedsAnswer, events.ZeroCommit
	claimed.Predicate = func(seq []Event) bool {
		return claimsWorkPresentRE.MatchString(seq[len(seq)-1].Text)
	}
	return []Entry{lost, unrecoverable, elsewhere, claimed}
}

// r2UnexecutedToolCall is a zero-commit finish whose last assistant turn is only
// a call literal. Telling that from a real answer needs the transcript, so an
// agent investigates; the verbs are the whole grant (one corrective nudge, then
// one finish-wait, both done by relaunching the iteration). Launches disabled:
// there is no S0 event data showing it occurs yet.
func r2UnexecutedToolCall() Entry {
	return Entry{
		ID: "R2", Type: events.NeedsAnswer, Kind: events.ZeroCommit,
		Predicate: func(seq []Event) bool {
			return transcript.LooksLikeUnexecutedToolCall(seq[len(seq)-1].Text)
		},
		Executor: ExecutorAgent, Authority: AuthorityMedium, Verbs: []string{"relaunch"},
	}
}

// r5PromptNeverDelivered is a launch whose pane never took its initial prompt.
// The park alone is ambiguous, so the predicate wants a stalled launch-failed
// before it. The remedy retypes the full prompt once (the per-kind guard rail
// is the cap); when that cannot be delivered it closes the pane, so a reclaim
// does not collide with the leftover agent. Launches enabled: S0 emits the
// launch-failed it matches.
func r5PromptNeverDelivered() Entry {
	return Entry{
		ID: "R5", Type: events.NeedsRepair, Kind: events.AgentPromptStalled,
		Predicate: func(seq []Event) bool {
			return slices.ContainsFunc(seq[:len(seq)-1], func(e Event) bool {
				return e.Type == events.LaunchFailed && e.Kind == events.AgentPromptStalled
			})
		},
		Executor: ExecutorRule, Authority: AuthorityLow, Verbs: []string{"nudge", "close-pane"}, Enabled: true,
		Remedy: func(f Failure, v Verbs) error {
			prompt, err := v.LaunchPrompt(f.Address)
			if err == nil {
				var res Result
				if res, err = v.Nudge(f.Address, prompt); err == nil && !res.Refused {
					return nil
				}
				if err == nil {
					err = fmt.Errorf("nudge refused: %s", res.Reason)
				}
			}
			if _, cerr := v.ClosePane(f.Address); cerr != nil {
				err = errors.Join(err, cerr)
			}
			return fmt.Errorf("retyping the prompt: %w", err)
		},
	}
}

// compactWaitTimedOutRE is a smart-zone-recovery-failed reason whose herdr
// wait ran out, as opposed to one the pane or herdr refused.
var compactWaitTimedOutRE = regexp.MustCompile(`"code":"timeout"|(?i)timed out`)

// FinishUpPrompt is what R7 types once the extended wait sees the compacted
// pane settle: the finish-up the iteration never got to send.
const FinishUpPrompt = "Your conversation was compacted after you exceeded the context window. " +
	"Please finish up quickly; if needed follow the `gx-implement` skill and create follow up tickets."

// r7CompactionTimedOut is an iteration that errored after its smart-zone
// compaction wait timed out. A timeout cannot tell a slow compaction from a
// dead pane, so the remedy waits once more, longer, then sends the finish-up.
// It never re-sends /compact: queued input cancels a running compaction. When
// the extended wait times out too the park stands and escalates; the per-kind
// guard rail is the cap on repeats. Launches enabled: S0 emits the event it
// matches, and it is one of the two failures that dominate the S0 data.
func r7CompactionTimedOut() Entry {
	return Entry{
		ID: "R7", Type: events.NeedsRepair, Kind: events.IterationError,
		Predicate: func(seq []Event) bool {
			for _, e := range slices.Backward(seq[:len(seq)-1]) {
				switch e.Type {
				case events.SmartZoneRecoveryFailed:
					if compactWaitTimedOutRE.MatchString(e.Reason) {
						return true
					}
				case events.IterationStarted:
					return false
				}
			}
			return false
		},
		Executor: ExecutorRule, Authority: AuthorityLow, Verbs: []string{"wait", "nudge"}, Enabled: true,
		Remedy: func(f Failure, v Verbs) error {
			res, err := v.Wait(f.Address)
			if err == nil && !res.Refused {
				res, err = v.Nudge(f.Address, FinishUpPrompt)
			}
			if err == nil && res.Refused {
				err = fmt.Errorf("refused: %s", res.Reason)
			}
			if err != nil {
				return fmt.Errorf("re-waiting for compaction: %w", err)
			}
			return nil
		},
	}
}

// r1Spin records a park/re-claim spin. The S0 spinning park already carries
// the count and window, so there is no predicate; the remedy does nothing
// because a nudge would only join the spin and the park already holds it.
// Launches enabled: S0 emits the event it matches.
func r1Spin() Entry {
	return Entry{
		ID: "R1", Type: events.NeedsRepair, Kind: events.Spinning,
		Executor: ExecutorRule, Authority: AuthorityLow, Enabled: true,
		Remedy: func(Failure, Verbs) error { return nil },
	}
}

// WithConfig applies the user's kill switch and per-entry disables.
func (c Catalog) WithConfig(enabled bool, disabled []string) Catalog {
	out := Catalog{Enabled: c.Enabled && enabled, Entries: slices.Clone(c.Entries)}
	for i := range out.Entries {
		if slices.Contains(disabled, out.Entries[i].ID) {
			out.Entries[i].Enabled = false
		}
	}
	return out
}

// EntryEnabled reports whether the catalog and its entry id are both on.
func (c Catalog) EntryEnabled(id string) bool {
	return c.Enabled && slices.ContainsFunc(c.Entries, func(e Entry) bool { return e.ID == id && e.Enabled })
}

// Match returns the first enabled entry matching seq, or false.
func (c Catalog) Match(seq []Event) (Entry, bool) {
	if !c.Enabled || len(seq) == 0 {
		return Entry{}, false
	}
	last := seq[len(seq)-1]
	for _, e := range c.Entries {
		if !e.Enabled || e.Type != last.Type || e.Kind != last.Kind {
			continue
		}
		if e.Predicate != nil && !e.Predicate(seq) {
			continue
		}
		return e, true
	}
	return Entry{}, false
}
