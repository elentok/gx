package herdrrunner

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/herdr"
)

// PromptNudgeGraceMs bounds each of PromptWithNudge's submit/re-wait attempts.
// PromptMaxNudges caps how many bare-Enter nudges it sends before giving up
// and returning the underlying poll-timeout error. PromptMaxRetypes caps how
// many times it falls back to resubmitting opts.Text in full (see
// ErrStuckSubmission's doc comment) before giving up early instead of
// spending the rest of PromptMaxNudges on nudges that can't help either.
//
// A tighter grace window was too short for a cold-started agent to reach
// "working" under concurrent load — observed live as a hard failure quoting
// the literal `--timeout` this constant produces, even with plenty of budget
// left on the caller's own overall deadline (budgetMs caps every individual
// attempt to this constant regardless of that deadline).
const (
	PromptNudgeGraceMs = 45_000
	PromptMaxNudges    = 3
	PromptMaxRetypes   = 2
)

// ErrStuckSubmission is PromptWithNudge's sentinel for a pane that shows no
// change at all across PromptMaxRetypes full resubmissions of opts.Text —
// distinct from the underlying poll-timeout error so a caller can tell "the
// agent is slow to start, a bare nudge might still land" apart from "this
// pane genuinely never received anything and retyping the same text into it
// again won't help either," and retry against a fresh pane instead.
var ErrStuckSubmission = errors.New("submission never reached the pane")

// PromptWithNudge wraps herdr's AgentPrompt/AgentSendKeys/AgentWait/AgentRead
// to submit opts.Text while working around a submission that never actually
// gets typed into the pane (observed in production: the text only appears
// once something else, like an operator's own keypress, nudges herdr's
// terminal-state detection).
//
// It first detects *start*, not completion: it submits with a short grace
// timeout, waiting for either "working" or any of the caller's own Until
// states (a completion so fast it's observed before "working" ever is). If
// neither is observed in that window, it reads the pane's current text and
// compares it against the snapshot taken before the previous attempt: if the
// pane changed at all (the common case — the text landed but wasn't
// submitted), it sends a bare Enter keypress and re-waits. If the pane shows
// no change whatsoever (the text never reached it in the first place, so
// Enter alone has nothing to submit), it resubmits opts.Text in full instead
// — up to PromptMaxRetypes times — rather than nudging blind. Retrying either
// way is capped by PromptMaxNudges total attempts, and an unchanged pane
// that's already exhausted PromptMaxRetypes retypes gives up immediately with
// ErrStuckSubmission rather than spending its remaining nudge budget on Enter
// keypresses that can't do anything either.
//
// Once "working" is observed, start/nudge handling is done: this enters a
// completion phase that only waits (never nudges or resubmits) for one of
// opts.Until's caller-requested final states. Both phases are billed against
// a single deadline computed from opts.TimeoutMs at entry — the grace window
// each start/nudge attempt gets shrinks as that budget is spent, and
// completion is handed whatever's left, so a slow completion — e.g.
// "/compact", which can take minutes — gets the remainder of the caller's own
// timeout instead of the fixed grace window, and an already-expired deadline
// is reported as a timeout instead of silently becoming an unbounded wait.
// opts.TimeoutMs <= 0 means no deadline at all: start/nudge attempts still
// use the fixed grace window (bounded), but completion then waits with no
// timeout (unlimited).
func PromptWithNudge(
	prompt func(herdr.AgentPromptOptions) (herdr.Agent, error),
	sendKeys func(target string, keys ...string) error,
	wait func(herdr.AgentWaitOptions) (herdr.Agent, error),
	read func(target string, opts herdr.AgentReadOptions) (string, error),
	now func() time.Time,
) func(herdr.AgentPromptOptions) (herdr.Agent, error) {
	return func(opts herdr.AgentPromptOptions) (herdr.Agent, error) {
		if !opts.Wait {
			return prompt(opts)
		}

		startUntil := opts.Until
		if !slices.Contains(startUntil, "working") {
			startUntil = append([]string{"working"}, startUntil...)
		}

		hasDeadline := opts.TimeoutMs > 0
		var deadline time.Time
		if hasDeadline {
			deadline = now().Add(time.Duration(opts.TimeoutMs) * time.Millisecond)
		}

		// budgetMs returns the timeout for the next start/nudge attempt,
		// the fixed grace window capped to what remains of the caller's
		// deadline. ok is false once that deadline has already passed.
		budgetMs := func() (ms int, ok bool) {
			if !hasDeadline {
				return PromptNudgeGraceMs, true
			}
			remaining := deadline.Sub(now())
			if remaining <= 0 {
				return 0, false
			}
			if remaining > PromptNudgeGraceMs*time.Millisecond {
				return PromptNudgeGraceMs, true
			}
			return max(1, int(remaining/time.Millisecond)), true
		}

		graceMs, ok := budgetMs()
		if !ok {
			return herdr.Agent{}, fmt.Errorf("timed out waiting for agent to start: overall deadline exceeded")
		}
		submit := opts
		submit.Until = startUntil
		submit.TimeoutMs = graceMs

		// snapshot is best-effort: a read error is treated the same as "the
		// pane changed" (unchanged stays false below), since there's no
		// evidence to declare it stuck — falling back to the older,
		// safer bare-Enter-nudge behavior rather than resubmitting blind.
		snapshot, _ := read(opts.Target, herdr.AgentReadOptions{Source: "recent-unwrapped"})

		agent, err := prompt(submit)
		retypes := 0
		for attempt := 0; IsPollTimeout(err) && attempt < PromptMaxNudges; attempt++ {
			graceMs, ok := budgetMs()
			if !ok {
				break
			}

			after, readErr := read(opts.Target, herdr.AgentReadOptions{Source: "recent-unwrapped"})
			unchanged := readErr == nil && after == snapshot
			snapshot = after

			if unchanged {
				if retypes >= PromptMaxRetypes {
					return herdr.Agent{}, fmt.Errorf("%w after %d retries: %w", ErrStuckSubmission, retypes, err)
				}
				retypes++
				resubmit := opts
				resubmit.Until = startUntil
				resubmit.TimeoutMs = graceMs
				agent, err = prompt(resubmit)
				continue
			}

			if nudgeErr := sendKeys(opts.Target, "enter"); nudgeErr != nil {
				return herdr.Agent{}, fmt.Errorf("nudging stuck submission: %w", nudgeErr)
			}
			agent, err = wait(herdr.AgentWaitOptions{
				Target:    opts.Target,
				Until:     startUntil,
				TimeoutMs: graceMs,
			})
		}
		if err != nil {
			return agent, err
		}

		if slices.Contains(opts.Until, agent.AgentStatus) {
			return agent, nil
		}

		completionTimeoutMs := 0
		if hasDeadline {
			remaining := deadline.Sub(now())
			if remaining <= 0 {
				return agent, fmt.Errorf("timed out waiting for completion: overall deadline exceeded")
			}
			completionTimeoutMs = max(1, int(remaining/time.Millisecond))
		}

		return wait(herdr.AgentWaitOptions{
			Target:    opts.Target,
			Until:     opts.Until,
			TimeoutMs: completionTimeoutMs,
		})
	}
}

// IsPollTimeout reports whether err looks like a transient herdr wait
// failure worth nudging and retrying rather than aborting: either AgentWait's
// own timeout-elapsed failure ("timed out waiting for agent status"), or
// herdr's agent_prompt_stalled error (the pane went idle with no observed
// state change within herdr's own internal stall window).
func IsPollTimeout(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timed out") ||
		strings.Contains(msg, "agent_prompt_stalled") ||
		strings.Contains(msg, "no observed state change")
}

// MatchedRuleID reads a pane's matched_rule.id via explain, falling back to
// "unknown" when explain is nil, fails, or returns no rule id.
func MatchedRuleID(explain func(string) (herdr.AgentExplainResult, error), pane string) string {
	if explain == nil {
		return "unknown"
	}
	if res, err := explain(pane); err == nil && res.MatchedRuleID != "" {
		return res.MatchedRuleID
	}
	return "unknown"
}
