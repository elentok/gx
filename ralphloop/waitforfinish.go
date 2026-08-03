package ralphloop

import (
	"fmt"
	"strings"
	"time"

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/herdr"
)

// smartZonePollMs bounds each "wait for the agent to finish" poll tick, so a
// running iteration's context occupancy is checked against --smart-zone at
// roughly this cadence instead of only once the agent settles.
const smartZonePollMs = 30_000

// finishDebounceMs is how long waitForFinish and finishIteration each pause
// before re-checking a just-reached "finished" signal, and finishConfirmMs is
// how long that recheck waits for the agent to prove it's still finished.
// herdr's idle/done status reflects pane output settling, not a genuine
// end-of-turn signal, so an agent that briefly stops producing output mid-turn
// (e.g. between its last tool call and a commit) can look finished for an
// instant. Without this debounce the loop would declare the iteration done,
// mark it needs-info (no commits yet), and abandon the worktree/tab while the
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
// unlike rate-limit/needs-attention pauses, this never blocks the
// scheduler via Gate.pause.
func waitForFinish(d Deps, p launchAndPromptParams, sessionID string) error {
	smartZone := p.SmartZone
	if smartZone <= 0 {
		smartZone = defaultSmartZone
	}

	elapsedMs := 0
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

		until := append([]string{}, plainFinishStates...)
		if p.Agent == AgentCodex {
			until = append(until, "blocked")
		}
		agent, err := d.AgentWait(herdr.AgentWaitOptions{
			Target:    p.Pane,
			Until:     until,
			TimeoutMs: pollMs,
		})
		if err == nil {
			if p.Agent == AgentCodex && agent.AgentStatus == "blocked" {
				if limit, exhausted, limitErr := codexRateLimit(d, p.SessionCwd, sessionID); limitErr == nil && exhausted {
					if err := recoverCodexRateLimit(d, p, sessionID, limit); err != nil {
						return err
					}
					elapsedMs = 0
					continue
				}
				if err := waitForAttentionRecovery(d, p, sessionID); err != nil {
					return err
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
			if p.Agent == AgentClaude && d.ReadPaneRecent != nil {
				if text, rlErr := d.ReadPaneRecent(p.Pane); rlErr == nil {
					if token, matched := detectRateLimit(text); matched {
						if err := recoverClaudeRateLimit(d, p, sessionID, token); err != nil {
							return err
						}
						elapsedMs = 0
						continue
					}
				}
			}

			confirmed, err := confirmFinished(d, p.Pane, until)
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
			p.logLifecycleEvent(p.FinishEvent, sessionID)
			return nil
		}
		if !isPollTimeout(err) {
			return fmt.Errorf("waiting for agent to finish: %w", err)
		}
		elapsedMs += pollMs

		if p.Agent == AgentCodex {
			if limit, exhausted, limitErr := codexRateLimit(d, p.SessionCwd, sessionID); limitErr == nil && exhausted {
				if err := recoverCodexRateLimit(d, p, sessionID, limit); err != nil {
					return err
				}
				elapsedMs = 0
				continue
			}
		}

		occupancy, ok, occErr := contextOccupancy(d, p.Agent, p.SessionCwd, sessionID)
		if occErr == nil && ok {
			p.sink().ContextOccupancy(p.Ticket, occupancy)
		}
		if occErr != nil || !ok || occupancy <= smartZone {
			continue
		}

		if err := d.AgentSendKeys(p.Pane, "ctrl+c"); err != nil {
			return fmt.Errorf("interrupting %s after smart-zone breach: %w", p.Label, err)
		}
		reason := fmt.Sprintf("context occupancy %d exceeds --smart-zone %d", occupancy, smartZone)
		if err := recoverSmartZoneBreach(d, p, sessionID, reason, smartZone); err != nil {
			return err
		}
		elapsedMs = 0
	}
}

// smartZoneRecoveryTimeoutMs bounds recoverSmartZoneBreach's finish-up
// AgentPrompt call. herdr's own --wait already fails fast (within ~5s) if it
// never observes a state change after submission, but once it does observe
// one it waits indefinitely for --until to match — and a submission that
// never actually gets typed into the pane (observed in production: the
// text only appears once something else, like an operator's own keypress,
// nudges herdr's terminal-state detection) can wedge this call, and with it
// this iteration's whole goroutine, forever. Bounding it here means a
// persistent problem shows up as a repeated smart-zone breach on the same
// ticket instead of a stuck loop.
const smartZoneRecoveryTimeoutMs = 30_000

// smartZoneCompactTimeoutMs bounds recoverSmartZoneBreach's wait for the
// "/compact" command itself to finish (pane back to idle/done), as opposed
// to merely starting (pane reaching "working"). Compacting a near-full
// context can take minutes, well past smartZoneRecoveryTimeoutMs.
const smartZoneCompactTimeoutMs = 300_000

// recoverSmartZoneBreach compacts the conversation and re-prompts the agent
// to finish up after a smart-zone breach, deliberately never calling
// Gate.pause: the scheduler keeps claiming and running other tickets while
// this iteration recompacts. It reports progress through
// SmartZoneCompactStarted/SmartZoneFinishingUp/SmartZoneRecovered rather than
// IterationPaused/IterationResumed, since this is a phase change on a still-
// running iteration, not something an operator could ever "resume" — see
// PauseKind's own doc comment. Either AgentPrompt call timing out (see
// smartZoneRecoveryTimeoutMs) is treated as best-effort and logged rather
// than propagated as a hard error: crashing the whole Run() over a stuck
// compaction would take down every other running iteration with it, and the
// agent may well still finish on its own even without this nudge.
//
// The "/compact" prompt waits for the pane to reach idle/done — i.e. for the
// compaction to actually finish, not merely start — before the finish-up
// prompt is sent. Waiting only for "working" let the finish-up text land
// mid-compaction and get swallowed as fresh input, canceling the compaction
// (observed in production as "Compaction canceled."); the caller's smart-zone
// check would then still see the old, uncompacted occupancy and re-trigger
// another breach, repeating the cycle.
func recoverSmartZoneBreach(d Deps, p launchAndPromptParams, sessionID, reason string, smartZone int) error {
	p.sink().SmartZoneCompactStarted(p.Ticket)
	p.logAgentEvent(eventPausedSmartZone, sessionID, reason)

	if _, err := d.AgentPrompt(herdr.AgentPromptOptions{
		Target:    p.Pane,
		Text:      "/compact",
		Wait:      true,
		Until:     plainFinishStates,
		TimeoutMs: smartZoneCompactTimeoutMs,
	}); err != nil {
		p.sink().SmartZoneRecovered(p.Ticket)
		p.logAgentEvent(eventSmartZoneRecoveryFailed, sessionID, fmt.Sprintf("compacting %s after smart-zone breach: %v", p.Label, err))
		return nil
	}

	p.sink().SmartZoneFinishingUp(p.Ticket)

	finishText := fmt.Sprintf(
		"I stopped you because you exceeded %d tokens in the context window, I compacted the "+
			"conversation, please finish up quickly, if needed follow the instructions in the "+
			"`implement` skill and create follow up tickets",
		smartZone,
	)
	if _, err := d.AgentPrompt(herdr.AgentPromptOptions{
		Target:    p.Pane,
		Text:      finishText,
		Wait:      true,
		Until:     []string{"working"},
		TimeoutMs: smartZoneRecoveryTimeoutMs,
	}); err != nil {
		p.sink().SmartZoneRecovered(p.Ticket)
		p.logAgentEvent(eventSmartZoneRecoveryFailed, sessionID, fmt.Sprintf("re-prompting %s after smart-zone compact: %v", p.Label, err))
		return nil
	}

	p.sink().SmartZoneRecovered(p.Ticket)
	p.logLifecycleEvent(eventResumed, sessionID)
	return nil
}

// confirmFinished debounces a just-reached idle/done signal on pane: it
// pauses finishDebounceMs, then re-polls for up to finishConfirmMs to see
// whether the agent is still in one of until's finish states. A poll timeout
// (the agent went back to "working" in the meantime) means the original
// signal was a transient blip, not a real finish.
func confirmFinished(d Deps, pane string, until []string) (bool, error) {
	d.Sleep(finishDebounceMs * time.Millisecond)
	_, err := d.AgentWait(herdr.AgentWaitOptions{
		Target:    pane,
		Until:     until,
		TimeoutMs: finishConfirmMs,
	})
	if err == nil {
		return true, nil
	}
	if isPollTimeout(err) {
		return false, nil
	}
	return false, err
}

// recoverClaudeRateLimit pauses label for display, waits out the rate limit
// via waitForClaudeRateLimitReset (racing the reset deadline against a
// resume request rather than blocking through it), then resumes and
// re-prompts the agent to continue.
func recoverClaudeRateLimit(d Deps, p launchAndPromptParams, sessionID, token string) error {
	reason := "rate limit detected"
	if token != "" {
		reason = fmt.Sprintf("rate limit detected, resets %s", token)
	}
	p.Gate.pause(p.Label, reason)
	p.sink().IterationPaused(p.Label, PauseRateLimit, reason)
	p.logAgentEvent(eventPausedRateLimit, sessionID, reason)
	waitForClaudeRateLimitReset(d, p.Gate, p.Label, p.ResumeSignalPath, p.Pane, token)
	p.Gate.ForceResume(p.Label)
	p.sink().IterationResumed(p.Label, PauseRateLimit)
	p.logLifecycleEvent(eventResumed, sessionID)

	if _, err := d.AgentPrompt(herdr.AgentPromptOptions{
		Target: p.Pane,
		Text:   "continue",
		Wait:   true,
		Until:  []string{"working"},
	}); err != nil {
		return fmt.Errorf("re-prompting %s after rate-limit reset: %w", p.Label, err)
	}
	return nil
}

func recoverCodexRateLimit(d Deps, p launchAndPromptParams, sessionID string, limit codexsession.RateLimit) error {
	reason := fmt.Sprintf("Codex %s quota exhausted", limit.Quota)
	if !limit.ResetAt.IsZero() {
		reason += fmt.Sprintf(", resets %s", limit.ResetAt.UTC().Format(time.RFC3339))
	}
	p.Gate.pause(p.Label, reason)
	p.report("paused %s: %s; waiting for automatic reset\n", p.Label, reason)
	p.logAgentEvent(eventPausedRateLimit, sessionID, reason)
	waitForCodexRateLimitReset(d, p.SessionCwd, sessionID, limit)
	p.Gate.ForceResume(p.Label)
	p.report("resumed %s after Codex quota reset\n", p.Label)
	p.logLifecycleEvent(eventResumed, sessionID)

	agent, err := d.AgentWait(herdr.AgentWaitOptions{
		Target: p.Pane,
		Until:  []string{"idle", "done", "working", "blocked"},
	})
	if err != nil {
		return fmt.Errorf("re-observing %s after Codex quota reset: %w", p.Label, err)
	}
	if agent.AgentStatus != "blocked" {
		return nil
	}

	if _, err := d.AgentPrompt(herdr.AgentPromptOptions{
		Target: p.Pane,
		Text:   "continue",
		Wait:   true,
		Until:  []string{"working"},
	}); err != nil {
		return fmt.Errorf("re-prompting %s after Codex quota reset: %w", p.Label, err)
	}
	return nil
}

// waitForAttentionRecovery makes a Codex permission/intervention request
// durable while keeping its pane and worktree available. The scheduler stays
// paused until the pane returns to idle/done; a resume signal merely asks for
// an immediate recheck, so it cannot accidentally schedule work while Codex
// remains blocked.
func waitForAttentionRecovery(d Deps, p launchAndPromptParams, sessionID string) error {
	const reason = "Codex is waiting for operator intervention"
	if err := MarkNeedsAttentionWithReason(p.TicketPath, reason); err != nil {
		return fmt.Errorf("marking ticket needs-attention: %w", err)
	}
	p.Gate.pause(p.Label, reason)
	p.sink().IterationPaused(p.Label, PauseNeedsAttention, reason)
	p.logAgentEvent(eventNeedsAttention, sessionID, reason)

	for {
		agent, err := d.AgentWait(herdr.AgentWaitOptions{
			Target:    p.Pane,
			Until:     []string{"idle", "done"},
			TimeoutMs: smartZonePollMs,
		})
		if err == nil && agent.AgentStatus != "blocked" {
			if err := Claim(p.TicketPath); err != nil {
				return fmt.Errorf("restoring ticket to claimed: %w", err)
			}
			p.Gate.ForceResume(p.Label)
			p.sink().IterationResumed(p.Label, PauseNeedsAttention)
			p.logLifecycleEvent(eventResumed, sessionID)
			return nil
		}
		if err != nil && !isPollTimeout(err) {
			return fmt.Errorf("rechecking blocked agent: %w", err)
		}

		signaled, signalErr := d.ResumeSignaled(p.ResumeSignalPath)
		if signalErr != nil || !signaled {
			continue
		}
		agent, err = d.AgentWait(herdr.AgentWaitOptions{
			Target: p.Pane,
			Until:  []string{"idle", "done", "blocked"},
		})
		if err != nil {
			return fmt.Errorf("manually rechecking blocked agent: %w", err)
		}
		if agent.AgentStatus == "blocked" {
			p.report("%s still needs attention\n", p.Label)
			continue
		}
		if err := Claim(p.TicketPath); err != nil {
			return fmt.Errorf("restoring ticket to claimed: %w", err)
		}
		p.Gate.ForceResume(p.Label)
		p.sink().IterationResumed(p.Label, PauseNeedsAttention)
		p.logLifecycleEvent(eventResumed, sessionID)
		return nil
	}
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

func codexRateLimit(d Deps, cwd, sessionID string) (codexsession.RateLimit, bool, error) {
	if sessionID == "" || d.ReadCodexRateLimit == nil {
		return codexsession.RateLimit{}, false, nil
	}
	return d.ReadCodexRateLimit(cwd, sessionID)
}

// isPollTimeout reports whether err looks like AgentWait's own
// timeout-elapsed failure (herdr's "timed out waiting for agent status"),
// as opposed to a genuine failure that should abort the loop instead of
// looping back for another poll tick.
func isPollTimeout(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "timed out")
}
