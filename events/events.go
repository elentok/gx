// Package events owns the run-log event contract: the wire codes for event
// types, the closed enum of failure/recovery kinds, which events must carry a
// kind, and the size cap that keeps every append atomic. The ralphloop event
// log writes through it, and the SSE stream will serialize the same codes.
package events

import (
	"fmt"
	"slices"
	"strings"
)

// Type is an event's wire code (the "type" field of a run-log line). The
// strings are fixed: renaming one breaks readers of existing logs.
type Type string

// Event types written by the event log today.
const (
	IterationStarted           Type = "iteration-started"
	IterationFinished          Type = "iteration-finished"
	CherryPicked               Type = "cherry-picked"
	LandDeferred               Type = "land-deferred"
	ConflictHit                Type = "conflict-hit"
	ConflictResolved           Type = "conflict-resolved"
	PausedSmartZone            Type = "paused-smart-zone"
	SmartZoneRecoveryFailed    Type = "smart-zone-recovery-failed"
	SmartZoneWaitExpired       Type = "smart-zone-wait-expired"
	SmartZoneGateReleased      Type = "smart-zone-gate-released"
	BackgroundTaskGateHeld     Type = "background-task-gate-held"
	BackgroundTaskGateReleased Type = "background-task-gate-released"
	BackgroundTaskGateExpired  Type = "background-task-gate-expired"
	PausedRateLimit            Type = "paused-rate-limit"
	Resumed                    Type = "resumed"
	NeedsAnswer                Type = "needs-answer"
	Commitless                 Type = "commitless"
	NeedsRepair                Type = "needs-repair"
	DepsInstalled              Type = "deps-installed"
	SchedulerScan              Type = "scheduler-scan"
	NotificationsConfigured    Type = "notifications-configured"
	NotificationSent           Type = "notification-sent"
	NotificationFailed         Type = "notification-failed"
	NotificationDegraded       Type = "notification-degraded"
	NotificationSuppressed     Type = "notification-suppressed"
	ManualLand                 Type = "manual-land"
	TicketReset                Type = "ticket-reset"
)

// Event types added by the orchestrator-daemon stages. Fixed now so S0 ships
// with the final wire codes.
const (
	LaunchFailed      Type = "launch-failed"
	Reclaimed         Type = "reclaimed"
	HerdrUnavailable  Type = "herdr-unavailable"
	HerdrAvailable    Type = "herdr-available"
	Submitted         Type = "submitted"
	SubmitRefused     Type = "submit-refused"
	Deadlocked        Type = "deadlocked"
	RecoveryMatched   Type = "recovery-matched"
	RecoveryApplied   Type = "recovery-applied"
	RecoveryProposed  Type = "recovery-proposed"
	RecoveryEscalated Type = "recovery-escalated"

	BudgetThresholdCrossed Type = "budget-threshold-crossed"
	BudgetSoftLimitPaused  Type = "budget-soft-limit-paused"
	BudgetHardLimitKilled  Type = "budget-hard-limit-killed"

	// Stream-only: published on the SSE stream, never written to the log.
	TicketChanged        Type = "ticket-changed"
	ExplainVerdictChange Type = "explain-verdict-change"
)

// Kind is the closed enum of failure/recovery causes. Catalog signatures
// match on (type, kind), never on the human-prose reason.
type Kind string

const (
	AgentNameTaken Kind = "agent_name_taken"
	AgentPaneBusy  Kind = "agent_pane_busy"
	// AgentPromptStalled is herdr's agent_prompt_stalled: the initial prompt
	// never reached the pane.
	AgentPromptStalled Kind = "agent_prompt_stalled"
	ZeroCommit         Kind = "zero-commit"
	BlockedPane        Kind = "blocked-pane"
	SelfReported       Kind = "self-reported"
	HandleMismatch     Kind = "handle-mismatch"
	RetryExhausted     Kind = "retry-exhausted"
	Spinning           Kind = "spinning"
	BudgetKilled       Kind = "budget-killed"
	AmbiguousLand      Kind = "ambiguous-land"
	AllParked          Kind = "all-parked"
	BlockedCycle       Kind = "blocked-cycle"
	DuplicateLive      Kind = "duplicate-live"
	// IterationError is the loop's catch-all: an iteration or land failed
	// with an error no more specific kind names.
	IterationError Kind = "iteration-error"
	// ManualPark is a person parking a ticket through `gx server tickets park`.
	ManualPark Kind = "manual-park"
	// AmbiguousBase is a claim that found two or more unlanded blockers and no
	// base: to choose between them.
	AmbiguousBase Kind = "ambiguous-base"
)

// kindCauseHerdr lists the kinds a herdr outage can cause. It is an attribute
// of the kind (the outage fold holds on it), never an event field.
var kindCauseHerdr = map[Kind]bool{
	AgentNameTaken: true,
	AgentPaneBusy:  true,
	HandleMismatch: true,
}

// kinds is the closed set; Valid checks membership.
var kinds = map[Kind]bool{
	AgentNameTaken: true, AgentPaneBusy: true, AgentPromptStalled: true, ZeroCommit: true, BlockedPane: true,
	SelfReported: true, HandleMismatch: true, RetryExhausted: true, Spinning: true,
	BudgetKilled: true, AmbiguousLand: true, AllParked: true, BlockedCycle: true,
	DuplicateLive: true, IterationError: true, ManualPark: true, AmbiguousBase: true,
}

// Kinds returns every kind in the enum, sorted so the CLI verb that publishes
// them has stable output.
func Kinds() []Kind {
	out := make([]Kind, 0, len(kinds))
	for k := range kinds {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// Valid reports whether k is in the closed enum.
func (k Kind) Valid() bool { return kinds[k] }

// CauseHerdr reports whether a herdr outage can cause this kind.
func (k Kind) CauseHerdr() bool { return kindCauseHerdr[k] }

// kindRequired lists the types that must carry a valid kind: the failure and
// recovery events introduced with the contract. Older failure types
// (needs-repair, needs-answer, commitless, ...) predate it; their call sites
// don't pass a kind yet, so a kind on them is validated if present but not
// demanded until they migrate.
var kindRequired = map[Type]bool{
	LaunchFailed:      true,
	SubmitRefused:     true,
	Deadlocked:        true,
	RecoveryMatched:   true,
	RecoveryApplied:   true,
	RecoveryProposed:  true,
	RecoveryEscalated: true,
}

// KindRequired reports whether t is a failure or recovery event that must
// carry a kind.
func KindRequired(t Type) bool { return kindRequired[t] }

// Validate checks the contract for an event of type t with the given kind:
// kind is mandatory on failure/recovery events, and any kind present must be
// in the closed enum.
func Validate(t Type, k Kind) error {
	if t == "" {
		return fmt.Errorf("event has no type")
	}
	if k == "" {
		if KindRequired(t) {
			return fmt.Errorf("event %q requires a kind", t)
		}
		return nil
	}
	if !k.Valid() {
		return fmt.Errorf("event %q has unknown kind %q", t, k)
	}
	return nil
}

// MaxLineBytes is the size cap for one run-log line, newline included. POSIX
// only keeps a single O_APPEND write(2) from interleaving with another
// writer's when it is at most PIPE_BUF-ish, so appends take no on-disk lock
// and rely on staying under this.
const MaxLineBytes = 4096

// Fit marshals an event and, while the line (plus newline) exceeds
// MaxLineBytes, shortens the given free-text fields in order — put reason
// first so it is cut before anything else. marshal must read the fields
// through the same pointers. It errors only if the line is still too big with
// every field emptied.
func Fit(marshal func() ([]byte, error), truncate ...*string) ([]byte, error) {
	data, err := marshal()
	if err != nil {
		return nil, err
	}
	for _, field := range truncate {
		// A raw byte encodes to between 1 and 6 JSON bytes, so cutting
		// excess/6 raw bytes never overshoots; re-measure and repeat.
		for len(data)+1 > MaxLineBytes && *field != "" {
			cut := max((len(data)+1-MaxLineBytes)/6, 1)
			keep := max(len(*field)-cut, 0)
			*field = strings.ToValidUTF8((*field)[:keep], "")
			if data, err = marshal(); err != nil {
				return nil, err
			}
		}
	}
	if len(data)+1 > MaxLineBytes {
		return nil, fmt.Errorf("event line is %d bytes, over the %d cap", len(data)+1, MaxLineBytes)
	}
	return data, nil
}
