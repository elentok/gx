package ralphloop

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/transcript"
)

// smartZonePollMs bounds each "wait for the agent to finish" poll tick, so a
// running iteration's context occupancy is checked against --smart-zone at
// roughly this cadence instead of only once the agent settles.
const smartZonePollMs = 30_000

// waitForFinishStates are the states that end one waitForFinish poll. Blocked
// is among them because it needs its own quota/park handling.
var waitForFinishStates = append(append([]agentrunner.State{}, runnerFinishStates...), agentrunner.StateBlocked)

// finishDebounceMs is how long waitForFinish and finishIteration each pause
// before re-checking a just-reached "finished" signal, and finishConfirmMs is
// how long that recheck waits for the agent to prove it's still finished.
// herdr's idle/done status reflects pane output settling, not a genuine
// end-of-turn signal, so an agent that briefly stops producing output mid-turn
// (e.g. between its last tool call and a commit) can look finished for an
// instant. Without this debounce the loop would declare the iteration done,
// mark it needs-answer (no commits yet), and abandon the worktree/tab while the
// agent went on to actually finish and commit — orphaning real, landed work.
const (
	finishDebounceMs = 3_000
	finishConfirmMs  = 2_000
)

// waitForFinish polls Pane until it reaches idle or done, checking the
// session's current context occupancy against SmartZone on every poll tick
// that times out rather than settling. A breach interrupts the pane
// (Ctrl-C, not killed), then auto-recovers via recoverSmartZoneBreach
// (compact + finish-up re-prompt) and falls back into normal polling —
// unlike rate-limit/needs-repair pauses, this never blocks the
// scheduler via Gate.pause.
func waitForFinish(d Deps, p launchAndPromptParams, sessionID string) error {
	smartZone := p.SmartZone
	if smartZone <= 0 {
		smartZone = defaultSmartZone
	}

	elapsedMs := 0
	// recovery holds the smart-zone-breach-and-recovery sub-state (gated
	// give-up bookkeeping and the post-recovery blocked-pane guard) apart from
	// the blocked-pane / rate-limit / gated-give-up branches below — see its
	// own doc comment.
	recovery := newSmartZoneRecovery(smartZone)
	for {
		pollMs := smartZonePollMs
		if p.FinishTimeoutMs > 0 {
			remaining := p.FinishTimeoutMs - elapsedMs
			if remaining <= 0 {
				return fmt.Errorf("waiting for agent to finish: timed out after %dms", p.FinishTimeoutMs)
			}
			if remaining < pollMs {
				pollMs = remaining
			}
		}

		status, err := d.Runner.Wait(p.session(), waitForFinishStates, time.Duration(pollMs)*time.Millisecond)
		if err == nil {
			if status.State == agentrunner.StateBlocked {
				if recovery.consumeJustRecovered() {
					elapsedMs = 0
					continue
				}
				if p.Agent == AgentCodex {
					resetAt, limited, _, limitErr := runnerRateLimit(d, p)
					if limitErr != nil {
						return fmt.Errorf("detecting %s Codex quota: %w", p.Label, limitErr)
					}
					if limited {
						if err := recoverCodexRateLimit(d, p, sessionID, resetAt); err != nil {
							return err
						}
						elapsedMs = 0
						continue
					}
				}
				parked, err := parkOnBlockedPane(d, p, sessionID)
				if err != nil {
					return err
				}
				if parked {
					return errBlockedPaneParked
				}
				elapsedMs = 0
				continue
			}

			// Claude has no structured "rate limited" status of its own: a
			// real hit just looks like the pane going idle, same as an
			// ordinary finish. So check for the rate-limit message here,
			// once the pane has actually stopped — not on every poll tick
			// while it's still working, which is what caused false alarms
			// from stale "approaching the limit" mentions still visible in
			// scrollback mid-turn.
			// A read failure here is not worth failing a finish over: the
			// ordinary finish checks below run regardless. Codex quota is
			// deliberately not acted on here, only its context exhaustion.
			resetAt, limited, evidence, _ := runnerRateLimit(d, p)
			if p.Agent == AgentCodex && evidence != "" {
				if err := d.Runner.Interrupt(p.session()); err != nil {
					return fmt.Errorf("interrupting %s after Codex context exhaustion: %w", p.Label, err)
				}
				if err := recoverOrFailCodexContextExhaustion(d, p, sessionID, evidence, smartZone); err != nil {
					return err
				}
				elapsedMs = 0
				continue
			}

			if p.Agent == AgentClaude && limited {
				if err := recoverClaudeRateLimit(d, p, sessionID, resetAt); err != nil {
					return err
				}
				elapsedMs = 0
				continue
			}

			confirmed, err := confirmFinished(d, p.session(), waitForFinishStates)
			if err != nil {
				return fmt.Errorf("confirming %s finished: %w", p.Label, err)
			}
			if !confirmed {
				// The agent went back to work in the debounce window (see
				// finishDebounceMs): this was a transient idle blip, not a real
				// finish, so keep waiting instead of declaring victory early.
				elapsedMs = 0
				continue
			}
			if !recovery.pendingUnresolved(d, p, sessionID) {
				finished, err := waitForBackgroundTasks(d, p, sessionID, waitForFinishStates, &elapsedMs)
				if err != nil {
					return err
				}
				if !finished {
					// The gate released, but the recheck found the agent back at
					// work reacting to the background task's result: not a real
					// finish, same as a transient idle blip above.
					elapsedMs = 0
					continue
				}
				p.logLifecycleEvent(p.FinishEvent, sessionID)
				return nil
			}
			// The last recovery gave up gated and the transcript still records no
			// boundary, so this "finished" is that same uncorroborated claim
			// arriving on the other poll kind — not a finish. Returning here would
			// close the ticket and abandon the worktree/tab mid-compaction, the
			// orphaning the gate exists to prevent. Give the transcript one more
			// poll interval (the compaction may simply be slow), then count a
			// still-silent one as another gated give-up, which is what keeps
			// maxConsecutiveGatedGiveUps reachable against a pane that answers
			// every poll kind idle. Re-prompting instead would type into a pane
			// that may still be compacting.
			d.Sleep(smartZonePollMs * time.Millisecond)
			elapsedMs += smartZonePollMs
			if !recovery.pendingUnresolved(d, p, sessionID) {
				finished, err := waitForBackgroundTasks(d, p, sessionID, waitForFinishStates, &elapsedMs)
				if err != nil {
					return err
				}
				if !finished {
					elapsedMs = 0
					continue
				}
				p.logLifecycleEvent(p.FinishEvent, sessionID)
				return nil
			}
			if recovery.recordGatedGiveUp() {
				return gatedGiveUpsExhausted(p.Label, recovery.gatedGiveUps, errCompactNeverConfirmed)
			}
			continue
		}
		if !errors.Is(err, agentrunner.ErrTimeout) {
			return fmt.Errorf("waiting for agent to finish: %w", err)
		}
		elapsedMs += pollMs

		if p.Agent == AgentCodex {
			resetAt, limited, evidence, checkErr := runnerRateLimit(d, p)
			if checkErr != nil {
				return fmt.Errorf("detecting %s Codex quota: %w", p.Label, checkErr)
			}
			if limited {
				if err := recoverCodexRateLimit(d, p, sessionID, resetAt); err != nil {
					return err
				}
				elapsedMs = 0
				continue
			}
			if evidence != "" {
				if err := d.Runner.Interrupt(p.session()); err != nil {
					return fmt.Errorf("interrupting %s after Codex context exhaustion: %w", p.Label, err)
				}
				if err := recoverOrFailCodexContextExhaustion(d, p, sessionID, evidence, smartZone); err != nil {
					return err
				}
				elapsedMs = 0
				continue
			}
		}

		// A gated give-up (the pane claimed completion the transcript never
		// corroborated) is a failed recovery, not a failed iteration: the agent
		// is still alive and may well compact on its own, so keep polling and
		// let the next breach try again. Every other error still aborts.
		breached, err := recovery.checkAndRecover(d, p, sessionID)
		if err != nil {
			return err
		}
		if breached {
			elapsedMs = 0
		}
	}
}

// smartZoneCompactTimeoutMs bounds recoverSmartZoneBreach's wait for the
// "/compact" command itself to finish (pane back to idle/done), as opposed
// to merely starting (pane reaching "working"). Compacting a near-full
// context can take minutes. Once this
// elapses without the pane confirming completion, waitForCompactionSignal
// starts consulting the transcript's compaction-boundary signal on every
// further poll tick (see smartZoneCompactExtendedTimeoutMs) instead of
// declaring failure immediately — a herdr pane-status observation gap is not
// the same thing as compaction actually being stuck.
const smartZoneCompactTimeoutMs = 300_000

// smartZoneCompactExtendedTimeoutMs is the outer bound recoverSmartZoneBreach
// gives a compact that's past smartZoneCompactTimeoutMs but whose transcript
// keeps showing no new compaction-boundary line either — i.e. neither signal
// has confirmed completion. Only once this elapses is the compact treated as
// a genuine, not merely slow, failure.
const smartZoneCompactExtendedTimeoutMs = 600_000

// errCompactNeverConfirmed marks the one recovery failure that means "the pane
// kept claiming the compaction was done and the transcript never agreed" — as
// opposed to a transport error, an unconfirmed submission, or a finish-up
// prompt that never landed, which are all equally "recovery failed" from
// outside. waitForFinish absorbs this one and keeps polling; every other error
// still aborts the iteration.
var errCompactNeverConfirmed = errors.New("compaction never confirmed by the transcript")

// maxConsecutiveGatedGiveUps bounds how many gated give-ups one iteration may
// absorb before waitForFinish stops retrying and escalates with
// errCompactRecoveryExhausted. Absorbing them without a bound never
// terminates: recovery gives up at the extended bound, the poll loop resets
// its elapsed counter, occupancy is still over the smart zone, and the whole
// cycle repeats forever with the finish-up prompt — the one thing that would
// end the iteration — never sent. The bound deliberately does not fall back to
// sending that prompt: doing so would reintroduce the compaction cancellation
// the gate exists to prevent, merely rate-limited. A stuck agent an operator
// can see beats an agent quietly destroying its own compactions.
//
// The count is per-iteration and consecutive: any successful recovery resets
// it, so give-ups an hour apart in a long healthy iteration never escalate.
const maxConsecutiveGatedGiveUps = 2

// errCompactRecoveryExhausted marks an iteration abandoned because compaction
// recovery kept giving up gated. It needs an operator, not another retry, which
// is why it leaves waitForFinish as an error: that routes it through Run's
// per-result handling (see loop.go), which persists the ticket
// needs-repair with this error's text as the reason. Both this message and
// the errCompactNeverConfirmed it wraps are load-bearing there — the operator
// reading the ticket needs to know the agent's own /compact never completed,
// not merely that some recovery failed.
var errCompactRecoveryExhausted = errors.New("smart-zone compaction recovery exhausted")

// gatedGiveUpsExhausted builds the escalation error for an iteration that hit
// maxConsecutiveGatedGiveUps, wrapping cause so the operator reading the
// needs-repair ticket still sees that it was the agent's own /compact that
// never completed.
func gatedGiveUpsExhausted(label string, giveUps int, cause error) error {
	return fmt.Errorf("%s: %w after %d consecutive attempts: %w",
		label, errCompactRecoveryExhausted, giveUps, cause)
}

// recoverSmartZoneBreach compacts the conversation and re-prompts the agent
// to finish up after a smart-zone breach, deliberately never calling
// Gate.pause: the scheduler keeps claiming and running other tickets while
// this iteration recompacts. It reports progress through
// SmartZoneCompactStarted/SmartZoneFinishingUp/SmartZoneRecovered rather than
// IterationPaused/IterationResumed, since this is a phase change on a still-
// running iteration, not something an operator could ever "resume" — see
// PauseKind's own doc comment. Either Prompt failing is treated as
// best-effort and logged rather than propagated as a hard error: crashing the
// whole Run() over a stuck compaction would take down every other running
// iteration with it, and the agent may well still finish on its own even
// without this nudge.
//
// Runner.Prompt("/compact") returns once the compaction started, having
// waited out any confirmation the agent asks for and checked the submission
// rendered. Recovery then waits for the compaction to actually finish before
// sending the finish-up prompt: sending it as soon as the turn started let the
// finish-up text land mid-compaction and get swallowed as fresh input,
// canceling compaction. recoverSmartZoneBreach's bool return reports whether
// the compact/finish-up contract actually completed (true) or was abandoned
// as best-effort (false) — callers that must not silently treat a failed
// recovery as a normal finish (see recoverOrFailCodexContextExhaustion) need
// that distinction; the plain proactive smart-zone breach caller still treats
// both outcomes the same way and ignores it.
//
// The compact-completion wait polls in smartZonePollMs ticks (via
// waitForCompactionSignal) rather than blocking on one long Wait, so a
// pane-status wait that times out past smartZoneCompactTimeoutMs can still be
// confirmed successful from the transcript's compaction-boundary signal
// instead of being misreported as a failure — see waitForCompactionSignal's
// doc comment.
func recoverSmartZoneBreach(d Deps, p launchAndPromptParams, sessionID, reason string, smartZone int) (bool, error) {
	p.sink().SmartZoneCompactStarted(p.Ticket)
	p.logAgentEvent(string(events.PausedSmartZone), sessionID, reason)

	baseline := newStickyBaseline(d, p, sessionID)

	err := d.Runner.Prompt(p.session(), "/compact")
	completion := compactPaneConfirmed
	if err == nil {
		completion, err = waitForCompactionSignal(d, p, sessionID, runnerFinishStates, baseline)
	}
	if err != nil {
		p.sink().SmartZoneRecovered(p.Ticket)
		p.logAgentEvent(string(events.SmartZoneRecoveryFailed), sessionID, fmt.Sprintf("compacting %s after smart-zone breach: %v", p.Label, err))
		if errors.Is(err, errCompactNeverConfirmed) {
			return false, err
		}
		return false, nil
	}
	switch completion {
	case compactTimeoutConfirmed:
		p.logAgentEvent(string(events.SmartZoneWaitExpired), sessionID, fmt.Sprintf("compact wait for %s expired but the transcript confirmed compaction completed", p.Label))
	case compactGateConfirmed:
		p.logAgentEvent(string(events.SmartZoneGateReleased), sessionID, fmt.Sprintf("the pane reported %s finished compacting before the transcript did; the gate held until the boundary landed", p.Label))
	}

	p.sink().SmartZoneFinishingUp(p.Ticket)

	finishText := fmt.Sprintf(
		"I stopped you because you exceeded %d tokens in the context window, I compacted the "+
			"conversation, please finish up quickly, if needed follow the instructions in the "+
			"`gx-implement` skill and create follow up tickets",
		smartZone,
	)
	if err := d.Runner.Prompt(p.session(), finishText); err != nil {
		p.sink().SmartZoneRecovered(p.Ticket)
		p.logAgentEvent(string(events.SmartZoneRecoveryFailed), sessionID, fmt.Sprintf("re-prompting %s after smart-zone compact: %v", p.Label, err))
		return false, nil
	}

	p.sink().SmartZoneRecovered(p.Ticket)
	p.logLifecycleEvent(string(events.Resumed), sessionID)
	return true, nil
}

// compactBoundaryState classifies what the transcript's compaction-boundary
// signal can say about an agent right now. The three states are deliberately
// not collapsed into a "have a baseline / don't" bool: only compactBoundary
// Unsupported may fail open (trust an idle pane report on its own), and
// folding compactBoundaryUnavailable into it would silently restore the bug
// this gate exists to prevent every time a transcript read blips.
type compactBoundaryState int

const (
	// compactBoundaryUnsupported: the agent has no boundary signal at all and
	// no amount of waiting will produce one.
	compactBoundaryUnsupported compactBoundaryState = iota
	// compactBoundaryConfirmed: a count was read and is authoritative.
	compactBoundaryConfirmed
	// compactBoundaryUnavailable: the signal exists for this agent but could
	// not be read right now — an unidentified session, a transcript that
	// doesn't exist yet, or a failed read.
	compactBoundaryUnavailable
)

type compactBoundarySnapshot struct {
	state compactBoundaryState
	count int
}

// readCompactBoundaries classifies one read of the agent's compaction-boundary
// count. An empty session id is unavailable rather than unsupported: the
// session exists and will be identified shortly, and calling it unsupported
// would fail a Claude agent open — the single most likely way one lands in the
// trust-the-pane bucket by accident.
func readCompactBoundaries(d Deps, agent AgentKind, cwd, sessionID string) compactBoundarySnapshot {
	if agent == AgentCodex || d.ReadCompactions == nil {
		return compactBoundarySnapshot{state: compactBoundaryUnsupported}
	}
	if sessionID == "" {
		return compactBoundarySnapshot{state: compactBoundaryUnavailable}
	}
	count, ok, err := d.ReadCompactions(cwd, sessionID)
	if err != nil || !ok {
		return compactBoundarySnapshot{state: compactBoundaryUnavailable}
	}
	return compactBoundarySnapshot{state: compactBoundaryConfirmed, count: count}
}

// gates reports whether the boundary count is authoritative for this agent,
// i.e. whether a pane-reported completion still needs the transcript's
// corroboration before it can end the compaction wait.
func (s compactBoundarySnapshot) gates() bool {
	return s.state != compactBoundaryUnsupported
}

// unresolved reports whether a compaction submitted at s still has nothing in
// the transcript to show for it. An agent with no boundary signal at all is
// never unresolved: there is no evidence to wait for, so nothing to withhold
// trust for.
func (s compactBoundarySnapshot) unresolved(d Deps, p launchAndPromptParams, sessionID string) bool {
	return s.gates() && !s.advancedPast(d, p, sessionID)
}

// advancedPast reports whether a fresh read proves a compaction boundary
// landed since s was taken. Anything short of a confirmed, higher count —
// including a read that fails now — is "not yet", never "close enough".
func (s compactBoundarySnapshot) advancedPast(d Deps, p launchAndPromptParams, sessionID string) bool {
	if s.state != compactBoundaryConfirmed {
		return false
	}
	now := readCompactBoundaries(d, p.Agent, p.SessionCwd, sessionID)
	return now.state == compactBoundaryConfirmed && now.count > s.count
}

// stickyBaseline carries what the compact-completion gate knows from before
// "/compact" was submitted: the boundary snapshot taken then, and the instant
// it was taken. It is never rebased mid-recovery. The gate's count predicate is
// "count is greater than baseline", so a baseline adopted after the compaction
// boundary has already landed is permanently greater than or equal to the
// count and the gate can never open again — re-reading a count to stand in for
// an unreadable baseline converts a transient read failure into a deadlock,
// reporting a genuinely successful compaction as a give-up and driving the
// ticket to needs-repair.
//
// An unavailable baseline therefore doesn't get a substitute count at all; it
// switches predicates instead, to "a boundary was written after since" (see
// boundaryLandedSince), which needs no pre-submission count to compare
// against. That reads an auto-compaction landing in the gap between the
// baseline read and submission as ours, which is the right call anyway: the
// context did get compacted, and the alternative — waiting for a second
// boundary that will never come — is the deadlock this type exists to rule
// out.
type stickyBaseline struct {
	snapshot compactBoundarySnapshot
	since    time.Time
}

// newStickyBaseline snapshots the boundary count and stamps since. Both must
// happen before "/compact" is submitted, so that any boundary this recovery
// causes is strictly newer than both.
func newStickyBaseline(d Deps, p launchAndPromptParams, sessionID string) *stickyBaseline {
	now := time.Now()
	if d.Now != nil {
		now = d.Now()
	}
	return &stickyBaseline{
		snapshot: readCompactBoundaries(d, p.Agent, p.SessionCwd, sessionID),
		since:    now,
	}
}

func (b *stickyBaseline) gates() bool {
	return b.snapshot.gates()
}

// advancedPast reports whether the transcript now proves this recovery's
// compaction landed.
func (b *stickyBaseline) advancedPast(d Deps, p launchAndPromptParams, sessionID string) bool {
	switch b.snapshot.state {
	case compactBoundaryConfirmed:
		return b.snapshot.advancedPast(d, p, sessionID)
	case compactBoundaryUnavailable:
		return b.boundaryLandedSince(d, p, sessionID)
	}
	return false
}

// boundaryLandedSince is the unavailable baseline's predicate. An agent with no
// boundary signal is excluded here as it is in readCompactBoundaries: its
// completion is decided by failing the gate open, never by this read. A read
// that fails proves nothing and holds the gate closed, like every other
// unconfirmed observation.
func (b *stickyBaseline) boundaryLandedSince(d Deps, p launchAndPromptParams, sessionID string) bool {
	if p.Agent == AgentCodex || d.ReadCompactionsAfter == nil || sessionID == "" {
		return false
	}
	count, ok, err := d.ReadCompactionsAfter(p.SessionCwd, sessionID, b.since)
	return err == nil && ok && count > 0
}

// compactCompletion records which of the three routes a confirmed compaction
// arrived by. The routes are kept apart because run-log.jsonl is the primary
// diagnostic instrument for this class of bug, and "the pane claimed to be
// idle mid-compaction" and "compaction genuinely took more than five minutes"
// call for opposite fixes.
type compactCompletion int

const (
	// compactPaneConfirmed: the pane reported completion and, where a boundary
	// signal exists at all, the transcript already agreed. The ordinary route,
	// worth no event of its own.
	compactPaneConfirmed compactCompletion = iota
	// compactGateConfirmed: the pane reported completion early, the gate
	// refused it, and the boundary landed within the compact timeout.
	compactGateConfirmed
	// compactTimeoutConfirmed: the pane-status wait kept timing out past
	// smartZoneCompactTimeoutMs and only the transcript ever confirmed.
	compactTimeoutConfirmed
)

// waitForCompactionSignal polls the session in smartZonePollMs ticks for one
// of until's states instead of blocking on a single long Wait, so a
// still-genuinely-running compact isn't indistinguishable from a stuck one
// just because the pane-status wait timed out.
//
// While baseline gates (see compactBoundarySnapshot), the transcript is
// authoritative in both directions. A pane that reports completion is believed
// only once the boundary count has advanced past baseline — Claude Code panes
// go idle mid-compaction, and trusting that report is what let the finish-up
// prompt land as queued input and cancel the compaction outright. A pane-status
// wait that keeps timing out past smartZoneCompactTimeoutMs is conversely still
// reported as success once the count advances, since a herdr observation gap is
// not a stuck compaction. A transcript that can't be read at all right now
// proves nothing either way, so it holds the gate closed and consumes the
// extended bound like any other unconfirmed tick. The baseline itself is fixed
// for the whole recovery and never re-read here — see stickyBaseline.
//
// The returned compactCompletion names the route taken. A gate that held at
// any point wins over the timeout route: the pane having lied about being idle
// is the more specific finding, and the one an investigator reading the run
// log needs to see.
//
// Only once smartZoneCompactExtendedTimeoutMs of accumulated time elapses with
// neither signal showing completion does this give up: with the pane's own
// timeout error if it was timing out, and with errCompactNeverConfirmed if it
// was reporting completion the transcript never corroborated. Returning the
// pane's nil error in that second case would let recovery walk straight into
// the finish-up prompt and reintroduce the bug ten minutes later.
func waitForCompactionSignal(
	d Deps, p launchAndPromptParams, sessionID string,
	until []agentrunner.State, baseline *stickyBaseline,
) (compactCompletion, error) {
	gateHeld := false
	elapsedMs := 0
	for {
		_, err := d.Runner.Wait(p.session(), until, smartZonePollMs*time.Millisecond)
		if err != nil && !errors.Is(err, agentrunner.ErrTimeout) {
			return compactPaneConfirmed, err
		}
		if err == nil {
			if !baseline.gates() || baseline.advancedPast(d, p, sessionID) {
				if gateHeld {
					return compactGateConfirmed, nil
				}
				return compactPaneConfirmed, nil
			}
			gateHeld = true
			// This wait returned immediately (that premature idle report is the
			// whole reason the gate exists), so it consumed none of the poll
			// interval the timeout branch below consumes inside Wait itself.
			// Pace it here instead — and only here: sleeping on both branches
			// would double-pace the timeout path and stretch the extended bound
			// to twice its wall-clock budget.
			d.Sleep(smartZonePollMs * time.Millisecond)
		}
		elapsedMs += smartZonePollMs

		if err != nil && elapsedMs >= smartZoneCompactTimeoutMs && baseline.advancedPast(d, p, sessionID) {
			if gateHeld {
				return compactGateConfirmed, nil
			}
			return compactTimeoutConfirmed, nil
		}

		if elapsedMs >= smartZoneCompactExtendedTimeoutMs {
			if err == nil {
				return compactPaneConfirmed, fmt.Errorf(
					"compacting %s: the pane kept reporting completion but the transcript recorded no compaction boundary: %w",
					p.Label, errCompactNeverConfirmed)
			}
			return compactPaneConfirmed, err
		}
	}
}

// confirmFinished debounces a just-reached idle/done signal on pane: it
// pauses finishDebounceMs, then re-polls for up to finishConfirmMs to see
// whether the agent is still in one of until's finish states. A poll timeout
// (the agent went back to "working" in the meantime) means the original
// signal was a transient blip, not a real finish.
func confirmFinished(d Deps, s agentrunner.Session, until []agentrunner.State) (bool, error) {
	d.Sleep(finishDebounceMs * time.Millisecond)
	_, err := d.Runner.Wait(s, until, finishConfirmMs*time.Millisecond)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, agentrunner.ErrTimeout) {
		return false, nil
	}
	return false, err
}

// waitForBackgroundTasks gates confirmFinished's conclusion on any
// outstanding-fresh backgrounded-shell-command marker in the Claude
// transcript: a pane that looks idle/done while a background task it started
// is still running is not actually finished, even though herdr's own status
// (and the finishDebounceMs re-check already applied by the caller) agree it
// looks so. Mirrors how compactBoundarySnapshot/stickyBaseline gate a
// premature idle report during smart-zone recovery — an unreadable/
// unsupported read (Codex, or any read failure) is never evidence, so it
// fails open with no markers to hold on.
//
// A held gate's release only proves that one background task's own
// completion notification landed — not that the agent's overall turn is
// over; seeing that result is exactly when an agent is likely to resume real
// work. So once a gate that actually held releases (or ages out), this
// re-runs confirmFinished before reporting finished=true, the same debounced
// idle check the caller already trusted once before the gate started
// holding. The returned bool tells the caller whether the pane is still
// idle after that recheck; false means treat this like any other transient
// idle blip and keep polling instead of declaring the iteration done.
//
// elapsedMs is the caller's own running total (see waitForFinish), threaded
// through by pointer so time spent holding here counts against a caller-set
// FinishTimeoutMs instead of resetting it, and it paces re-reads at
// smartZonePollMs instead of spinning — same reasoning as
// waitForCompactionSignal's own pacing.
func waitForBackgroundTasks(d Deps, p launchAndPromptParams, sessionID string, until []agentrunner.State, elapsedMs *int) (bool, error) {
	if p.Agent != AgentClaude || d.ReadBackgroundTasks == nil || sessionID == "" {
		return true, nil
	}

	held := map[string]bool{}
	gated := false
	for {
		reading, err := d.ReadBackgroundTasks(p.SessionCwd, sessionID)
		if err != nil {
			return true, nil
		}

		outstanding := false
		for _, m := range reading.Markers {
			switch m.Status {
			case transcript.BackgroundTaskOutstandingFresh:
				outstanding = true
				if !held[m.TaskID] {
					held[m.TaskID] = true
					gated = true
					p.logGateEvent(events.BackgroundTaskGateHeld, sessionID, m.TaskID, fmt.Sprintf("background task %s", m.TaskID))
				}
			case transcript.BackgroundTaskResolved:
				if held[m.TaskID] {
					delete(held, m.TaskID)
					p.logGateEvent(events.BackgroundTaskGateReleased, sessionID, m.TaskID, fmt.Sprintf("background task %s", m.TaskID))
				}
			case transcript.BackgroundTaskOutstandingAgedOut:
				if held[m.TaskID] {
					delete(held, m.TaskID)
					p.logGateEvent(events.BackgroundTaskGateExpired, sessionID, m.TaskID, fmt.Sprintf("background task %s", m.TaskID))
				}
			}
		}
		if outstanding && d.GateReleased != nil && d.GateReleased() {
			for id := range held {
				p.logGateEvent(events.BackgroundTaskGateReleased, sessionID, id, fmt.Sprintf("background task %s: recovery forced the release", id))
			}
			clear(held)
			outstanding = false
		}
		if !outstanding {
			if !gated {
				return true, nil
			}
			confirmed, err := confirmFinished(d, p.session(), until)
			if err != nil {
				return false, fmt.Errorf("confirming %s still finished after background task gate: %w", p.Label, err)
			}
			return confirmed, nil
		}

		pollMs := smartZonePollMs
		if p.FinishTimeoutMs > 0 {
			remaining := p.FinishTimeoutMs - *elapsedMs
			if remaining <= 0 {
				return false, fmt.Errorf("waiting for agent to finish: timed out after %dms", p.FinishTimeoutMs)
			}
			if remaining < pollMs {
				pollMs = remaining
			}
		}
		d.Sleep(time.Duration(pollMs) * time.Millisecond)
		*elapsedMs += pollMs
	}
}

// recoverClaudeRateLimit pauses label for display, waits out the rate limit
// via waitForClaudeRateLimitReset (racing the reset deadline against a
// resume request rather than blocking through it), then resumes and
// re-prompts the agent to continue.
func recoverClaudeRateLimit(d Deps, p launchAndPromptParams, sessionID string, resetAt time.Time) error {
	reason := "rate limit detected"
	if !resetAt.IsZero() {
		reason = fmt.Sprintf("rate limit detected, resets %s", resetAt.UTC().Format(time.RFC3339))
	}
	p.Gate.pause(p.Label, reason)
	p.sink().IterationPaused(p.Ticket, p.Label, PauseRateLimit, reason)
	p.logAgentEvent(string(events.PausedRateLimit), sessionID, reason)
	waitForClaudeRateLimitReset(d, p.Gate, p.session(), resetAt)
	p.sink().IterationResumed(p.Ticket, p.Label, PauseRateLimit)
	p.logLifecycleEvent(string(events.Resumed), sessionID)
	p.Gate.ForceResume(p.Label)

	if err := d.Runner.Prompt(p.session(), "continue"); err != nil {
		return fmt.Errorf("re-prompting %s after rate-limit reset: %w", p.Label, err)
	}
	return nil
}

func recoverCodexRateLimit(d Deps, p launchAndPromptParams, sessionID string, resetAt time.Time) error {
	reason := "Codex quota exhausted"
	if !resetAt.IsZero() {
		reason += fmt.Sprintf(", resets %s", resetAt.UTC().Format(time.RFC3339))
	}
	p.Gate.pause(p.Label, reason)
	p.sink().IterationPaused(p.Ticket, p.Label, PauseRateLimit, reason)
	p.logAgentEvent(string(events.PausedRateLimit), sessionID, reason)
	waitForCodexRateLimitReset(d, p.session(), resetAt)
	p.sink().IterationResumed(p.Ticket, p.Label, PauseRateLimit)
	p.logLifecycleEvent(string(events.Resumed), sessionID)
	p.Gate.ForceResume(p.Label)

	st, err := d.Runner.Status(p.session())
	if err != nil {
		return fmt.Errorf("re-observing %s after Codex quota reset: %w", p.Label, err)
	}
	if st.State != agentrunner.StateBlocked {
		return nil
	}

	return parkBlockedAfterCodexQuotaReset(p, sessionID, st.BlockedReason)
}

// parkBlockedAfterCodexQuotaReset handles a Codex pane that comes back
// blocked once its quota reset: unlike waitForClaudeRateLimitReset's plain
// "continue" re-prompt, a blocked pane here is sitting on its own dialog, not
// waiting for the iteration to resume — "continue" is a prompt, not a dialog
// answer, and gx never raised the dialog in the first place, so it must not
// be sent. There is also no trust_directory branch to try: the directory was
// already trusted at launch and gx was asleep for the reset, so the only
// answerable-dialog case (see ticket 01) can't occur here. This parks for a
// human instead, naming the unanswered dialog by the runner's
// Status.BlockedReason — unlike parkOnBlockedPane, whose park reason names no
// dialog at all.
func parkBlockedAfterCodexQuotaReset(p launchAndPromptParams, sessionID, blockedReason string) error {
	reason := fmt.Sprintf("%s came back blocked on dialog %q after a Codex quota reset; answer it in the pane", p.Label, blockedReason)
	p.parkBlockedPane(sessionID, reason)
	return errBlockedPaneParked
}

// blockedDwellMs is the fixed window waitForFinish waits, once, after first
// observing a pane blocked, before parkOnBlockedPane's single re-check. A
// momentary block (the agent clearing its own transient prompt) shouldn't
// park a healthy iteration, but this is a fixed window, not a settle timer:
// a pane that leaves and re-enters the blocked state inside it does not
// restart the wait, and nothing about the pane is observed again until the
// window ends.
const blockedDwellMs = 15_000

// errBlockedPaneParked is waitForFinish's sentinel return for a pane parked
// by parkOnBlockedPane: not a failure, but a signal to its callers
// (launchAndPrompt, runIteration, reattachIteration) to end the iteration
// without running the ordinary finish path (commit check, cherry-pick,
// worktree/tab cleanup) that assumes the agent actually finished — this
// pane's agent is still live, just waiting on a prompt nobody has answered
// yet.
var errBlockedPaneParked = errors.New("iteration parked: pane blocked on an unanswered prompt")

// parkOnBlockedPane waits out blockedDwellMs, then re-reads the session once via
// Runner.Status (a peek, not another Wait) and, only if it is still blocked
// at that instant, writes the pane-answered park (needs-answer, a reason, and
// a "## Needs Answer" stub, both naming p.Label — the pane is named by its
// iteration label, never a raw pane id, since a label still resolves after a
// restart or reattach and a pane id does not) and reports parked=true.
//
// No prompt is ever sent to the pane here: typing into a pane sitting on an
// operator's own pending dialog would be the destructive interrupt this path
// exists to avoid.
//
// This whole gate rests on herdr's pane monitor still recognizing a blocked
// Claude form as `agent_status: blocked` — that recognition lives outside
// this repo and can silently regress on a Claude Code or herdr upgrade. See
// docs/runbooks/blocked-form-regression-check.md for the (necessarily
// interactive, via `gx doctor --check-blocked-form`) way to check it.
func parkOnBlockedPane(d Deps, p launchAndPromptParams, sessionID string) (parked bool, err error) {
	d.Sleep(blockedDwellMs * time.Millisecond)

	st, err := d.Runner.Status(p.session())
	if err != nil {
		return false, fmt.Errorf("rechecking %s for park: %w", p.Label, err)
	}
	if st.State != agentrunner.StateBlocked {
		return false, nil
	}

	reason := fmt.Sprintf("%s is blocked on a prompt gx did not send; answer it in the pane", p.Label)
	p.parkBlockedPane(sessionID, reason)
	return true, nil
}

// parkBlockedPane routes a blocked-pane park through the single park path,
// keeping the agent context on the event.
func (p launchAndPromptParams) parkBlockedPane(sessionID, reason string) {
	park(p.sink(), parkRequest{
		ScratchDir: p.ScratchDir, EpicName: p.EpicName, Ticket: p.Ticket, Path: p.TicketPath,
		Type: events.NeedsAnswer, Kind: events.BlockedPane, Reason: reason,
		Event: Event{Agent: p.Agent, Pane: p.Pane, Tab: p.Tab, AgentSession: sessionID, Cwd: p.SessionCwd},
	})
}

// contextOccupancy reads the selected agent's own local session data. A
// missing observer is treated like incomplete session data, keeping the
// running iteration alive instead of falsely pausing it.
func contextOccupancy(d Deps, agent AgentKind, cwd, sessionID string) (int, bool, error) {
	if sessionID == "" {
		return 0, false, nil
	}
	if agent == AgentCodex {
		if d.ReadCodexContext == nil {
			return 0, false, nil
		}
		return d.ReadCodexContext(cwd, sessionID)
	}
	if d.ReadOccupancy == nil {
		return 0, false, nil
	}
	return d.ReadOccupancy(cwd, sessionID)
}

// smartZoneOccupancy reads the session's occupancy for one poll tick and
// reports separately what the tick may display and what it may decide a breach
// on. The two diverge in the window between a compaction landing and the
// agent's next turn: the transcript still holds only the pre-compaction
// figure, which is right to keep showing and wrong to breach on again — doing
// so interrupts the agent and starts a second /compact on top of the finish-up
// work the first recovery just asked for. Agents (Codex) and wirings without
// the staleness-aware reader fall back to the general read, where every found
// number is decidable.
func smartZoneOccupancy(d Deps, agent AgentKind, cwd, sessionID string) (occupancy int, found, decidable bool, err error) {
	if agent == AgentClaude && sessionID != "" && d.ReadOccupancyReading != nil {
		reading, err := d.ReadOccupancyReading(cwd, sessionID)
		if err != nil {
			return 0, false, false, err
		}
		return reading.Usage.Occupancy(), reading.Found, reading.Found && !reading.Stale, nil
	}
	occupancy, found, err = contextOccupancy(d, agent, cwd, sessionID)
	return occupancy, found, found, err
}

// emitContextOccupancy reads cwd/sessionID's current context occupancy and,
// if available, reports it via sink — the one extra immediate read
// IterationStarted/TicketReattached each trigger (see EventSink.
// ContextOccupancy) so a consumer never shows a misleading "0 tok" for up to
// smartZonePollMs after starting/reattaching. A missing/unreadable occupancy
// (occErr != nil or !ok, e.g. no session id yet) is silently skipped rather
// than emitting a misleading zero.
func emitContextOccupancy(d Deps, sink EventSink, agent AgentKind, identifier, cwd, sessionID string) {
	occupancy, ok, err := contextOccupancy(d, agent, cwd, sessionID)
	if err != nil || !ok {
		return
	}
	sink.ContextOccupancy(identifier, occupancy)
}

// sessionCompactions reads how many compaction boundaries the selected
// agent's transcript recorded. Codex sessions have no equivalent local
// signal today, so a Codex agent always reports ok=false rather than
// guessing; a missing observer or read failure is likewise treated as
// "unknown" (count 0, omitted from frontmatter) rather than blocking the
// ticket close it's stamped alongside.
func sessionCompactions(d Deps, agent AgentKind, cwd, sessionID string) (int, bool, error) {
	if sessionID == "" || agent == AgentCodex || d.ReadCompactions == nil {
		return 0, false, nil
	}
	return d.ReadCompactions(cwd, sessionID)
}

// runnerRateLimit asks the runner whether p's agent is rate limited.
// ErrContextExhausted is not a failure: it is returned as the evidence the
// adapter found, with limited false and a nil err.
func runnerRateLimit(d Deps, p launchAndPromptParams) (resetAt time.Time, limited bool, evidence string, err error) {
	resetAt, limited, err = d.Runner.RateLimit(p.session())
	if errors.Is(err, agentrunner.ErrContextExhausted) {
		return time.Time{}, false, strings.TrimPrefix(err.Error(), agentrunner.ErrContextExhausted.Error()+": "), nil
	}
	return resetAt, limited, "", err
}

// recoverOrFailCodexContextExhaustion runs the compact/finish-up contract for
// a classified native Codex context exhaustion and turns an incomplete
// recovery into a durable, actionable error instead of letting the caller
// fall back into ordinary polling. Without this, a recovery whose /compact or
// finish-up prompt never lands (the agent's context is already too far gone
// to process either) leaves the pane sitting idle post-interrupt with no
// further evidence of what happened — waitForFinish would then read that as
// a plain finish, and finishIteration would mark it done (if a stray commit
// happened to land) or generic needs-answer (if not), losing the exhaustion
// reason entirely. Returning an error here instead routes through the same
// path every other iteration-level failure takes (see Run's per-result
// handling in loop.go), which marks the ticket needs-repair with this
// specific reason and leaves its worktree/tab for inspection.
func recoverOrFailCodexContextExhaustion(d Deps, p launchAndPromptParams, sessionID, evidence string, smartZone int) error {
	reason := fmt.Sprintf("Codex context exhaustion detected: %s", evidence)
	recovered, err := recoverSmartZoneBreach(d, p, sessionID, reason, smartZone)
	if err != nil {
		return err
	}
	if !recovered {
		return fmt.Errorf("Codex context exhaustion recovery failed for %s: %s", p.Label, evidence)
	}
	return nil
}
