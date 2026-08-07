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
				limit, exhausted, limitErr := codexRateLimit(d, p.SessionCwd, sessionID, p.Pane)
				if limitErr != nil {
					return fmt.Errorf("detecting %s Codex quota: %w", p.Label, limitErr)
				}
				if exhausted {
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
			if p.Agent == AgentCodex {
				recovered, recoveryErr := recoverCodexContextExhaustion(d, p, sessionID, smartZone)
				if recoveryErr != nil {
					return recoveryErr
				}
				if recovered {
					elapsedMs = 0
					continue
				}
			}

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
			limit, exhausted, evidence, checkErr := codexQuotaOrContextExhaustion(d, p.SessionCwd, sessionID, p.Pane)
			if checkErr != nil {
				return fmt.Errorf("detecting %s Codex quota: %w", p.Label, checkErr)
			}
			if exhausted {
				if err := recoverCodexRateLimit(d, p, sessionID, limit); err != nil {
					return err
				}
				elapsedMs = 0
				continue
			}
			if evidence != "" {
				if err := d.AgentSendKeys(p.Pane, "ctrl+c"); err != nil {
					return fmt.Errorf("interrupting %s after Codex context exhaustion: %w", p.Label, err)
				}
				if err := recoverOrFailCodexContextExhaustion(d, p, sessionID, evidence, smartZone); err != nil {
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
		if _, err := recoverSmartZoneBreach(d, p, sessionID, reason, smartZone); err != nil {
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
// context can take minutes, well past smartZoneRecoveryTimeoutMs. Once this
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

// smartZoneCompactSubmitPollMs is the tick size for confirmCompactSubmitted's
// retry loop; smartZoneCompactSubmitTimeoutMs is its total budget. herdr's
// idle/done sample can be taken before Enter's effect has rendered "/compact"
// as submitted in the pane, so a completion signal alone doesn't mean the
// finish-up prompt is safe to send — it can still land concatenated with an
// unsubmitted "/compact". Each tick re-checks after a plain Sleep (not an
// AgentWait — the pane's status is irrelevant to whether "/compact" has
// rendered, and treating a "working" transition as a poll result would burn
// through the retry budget in one tick instead of pacing it) rather than
// sending a fresh keypress: a blind Enter here risks canceling a genuine
// in-progress compaction.
const (
	smartZoneCompactSubmitPollMs    = 5_000
	smartZoneCompactSubmitTimeoutMs = 30_000
)

// confirmCompactSubmitted reports whether "/compact" has actually rendered as
// submitted in pane, by reading its trailing line via AgentRead. A trailing
// line still reading "/compact" means Enter's effect on the pane hasn't
// rendered yet.
func confirmCompactSubmitted(d Deps, pane string) (bool, error) {
	out, err := d.AgentRead(pane, herdr.AgentReadOptions{Source: "recent-unwrapped"})
	if err != nil {
		return false, err
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	trailing := strings.TrimSpace(lines[len(lines)-1])
	return trailing != "/compact", nil
}

// confirmCompactSubmittedWithRetry gates entry into recoverSmartZoneBreach's
// finish-up phase behind confirmCompactSubmitted, bounded to
// smartZoneCompactSubmitTimeoutMs total. It never sends a nudge keypress or
// resubmits "/compact" — only sleeps between polls to give the pane a chance
// to render the submission.
func confirmCompactSubmittedWithRetry(d Deps, pane string) error {
	elapsedMs := 0
	for {
		submitted, err := confirmCompactSubmitted(d, pane)
		if err != nil {
			return err
		}
		if submitted {
			return nil
		}
		if elapsedMs >= smartZoneCompactSubmitTimeoutMs {
			return fmt.Errorf("/compact still unsubmitted in pane after %ds", smartZoneCompactSubmitTimeoutMs/1000)
		}
		d.Sleep(smartZoneCompactSubmitPollMs * time.Millisecond)
		elapsedMs += smartZoneCompactSubmitPollMs
	}
}

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
// prompt is sent. Codex may first report blocked while waiting for compact
// confirmation; that state belongs to this flow, so recovery observes the
// confirmation and compaction transitions passively instead of treating them
// as operator intervention. Waiting only for "working" let the finish-up text
// land mid-compaction and get swallowed as fresh input, canceling compaction.
// recoverSmartZoneBreach's bool return reports whether the compact/finish-up
// contract actually completed (true) or was abandoned as best-effort after
// one of its two failure branches (false) — callers that must not silently
// treat a failed recovery as a normal finish (see
// recoverOrFailCodexContextExhaustion) need that distinction; the plain
// proactive smart-zone breach caller still treats both outcomes the same way
// and ignores it.
//
// The compact-completion wait polls in smartZonePollMs ticks (via
// waitForCompactionSignal) rather than blocking on one long AgentPrompt/
// AgentWait call, so a pane-status wait that times out past
// smartZoneCompactTimeoutMs can still be confirmed successful from the
// transcript's compaction-boundary signal instead of being misreported as a
// failure — see waitForCompactionSignal's doc comment.
//
// Neither AgentPrompt call below reads the pane back to confirm "/compact" or
// the finish-up text was actually submitted before treating the wait as
// meaningful. That's safe only because every call here passes Wait: true:
// herdr's own --wait (as of 0.8.0) captures a state_change_seq baseline at
// submission and refuses to match Until against anything until that sequence
// actually advances, so a prompt sitting typed-but-unsubmitted in the pane
// (herdr's `agent prompt` sends the trailing Enter from a task delayed up to
// 300ms after returning, see herdr's AGENT_PROMPT_SUBMIT_DELAY) can't be
// mistaken for a state that predates it. This is herdr behavior, not
// something gx enforces — an AgentPrompt call added here without Wait: true
// would reintroduce the race this comment is warning about.
// compactSignalUnconfirmed reports whether the immediate "/compact"
// AgentPrompt result (err) needs a fallthrough to waitForCompactionSignal
// rather than being trusted as-is: either the prompt's own wait timed out, or
// it returned success but the transcript's compaction-boundary count hasn't
// advanced past baselineCompactions yet — a premature idle/done report, not
// proof the compact actually finished. A re-fetch error is treated the same
// as "no baseline to check against" (trust the immediate success), matching
// the pre-extraction behavior this replaces.
func compactSignalUnconfirmed(d Deps, p launchAndPromptParams, sessionID string, err error, baselineCompactions int, haveBaseline bool) bool {
	if err != nil {
		return isPollTimeout(err)
	}
	if !haveBaseline {
		return false
	}
	count, ok, readErr := sessionCompactions(d, p.Agent, p.SessionCwd, sessionID)
	return readErr == nil && ok && count <= baselineCompactions
}

func recoverSmartZoneBreach(d Deps, p launchAndPromptParams, sessionID, reason string, smartZone int) (bool, error) {
	p.sink().SmartZoneCompactStarted(p.Ticket)
	p.logAgentEvent(eventPausedSmartZone, sessionID, reason)

	baselineCompactions, haveBaseline, _ := sessionCompactions(d, p.Agent, p.SessionCwd, sessionID)

	compactStates := append(append([]string{}, plainFinishStates...), "blocked")
	agent, err := d.AgentPrompt(herdr.AgentPromptOptions{
		Target:    p.Pane,
		Text:      "/compact",
		Wait:      true,
		Until:     compactStates,
		TimeoutMs: smartZonePollMs,
	})
	expired := false
	if compactSignalUnconfirmed(d, p, sessionID, err, baselineCompactions, haveBaseline) {
		agent, expired, err = waitForCompactionSignal(d, p, sessionID, compactStates, smartZonePollMs, baselineCompactions, haveBaseline)
	}
	if err == nil && agent.AgentStatus == "blocked" {
		_, err = d.AgentWait(herdr.AgentWaitOptions{
			Target:    p.Pane,
			Until:     []string{"working"},
			TimeoutMs: smartZoneCompactTimeoutMs,
		})
		if err == nil {
			_, err = d.AgentWait(herdr.AgentWaitOptions{
				Target:    p.Pane,
				Until:     plainFinishStates,
				TimeoutMs: smartZonePollMs,
			})
			if err != nil && isPollTimeout(err) {
				_, expired, err = waitForCompactionSignal(d, p, sessionID, plainFinishStates, smartZonePollMs, baselineCompactions, haveBaseline)
			}
		}
	}
	if err != nil {
		p.sink().SmartZoneRecovered(p.Ticket)
		p.logAgentEvent(eventSmartZoneRecoveryFailed, sessionID, fmt.Sprintf("compacting %s after smart-zone breach: %v", p.Label, err))
		return false, nil
	}
	if expired {
		p.logAgentEvent(eventSmartZoneWaitExpired, sessionID, fmt.Sprintf("compact wait for %s expired but the transcript confirmed compaction completed", p.Label))
	}

	if err := confirmCompactSubmittedWithRetry(d, p.Pane); err != nil {
		p.sink().SmartZoneRecovered(p.Ticket)
		p.logAgentEvent(eventSmartZoneRecoveryFailed, sessionID, fmt.Sprintf("confirming /compact submitted for %s: %v", p.Label, err))
		return false, nil
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
		return false, nil
	}

	p.sink().SmartZoneRecovered(p.Ticket)
	p.logLifecycleEvent(eventResumed, sessionID)
	return true, nil
}

// waitForCompactionSignal polls Pane in smartZonePollMs ticks for one of
// until's states instead of blocking on a single long AgentWait call, so a
// still-genuinely-running compact isn't indistinguishable from a stuck one
// just because herdr's own pane-status wait timed out. startElapsedMs is the
// time already spent by the caller's own first poll tick before handing off
// here. Once total elapsed time passes smartZoneCompactTimeoutMs, each
// further tick that also times out additionally checks the transcript's
// compaction-boundary count (via sessionCompactions): a count higher than
// baselineCompactions means compaction actually completed since "/compact"
// was submitted, even though the pane-status wait never observed it — that's
// reported as success (transcriptConfirmed=true) rather than a failure. Only
// once smartZoneCompactExtendedTimeoutMs elapses with neither signal showing
// completion is the pane's timeout error returned as a genuine failure.
func waitForCompactionSignal(
	d Deps, p launchAndPromptParams, sessionID string,
	until []string, startElapsedMs int,
	baselineCompactions int, haveBaseline bool,
) (agent herdr.Agent, transcriptConfirmed bool, err error) {
	elapsedMs := startElapsedMs
	for {
		agent, err = d.AgentWait(herdr.AgentWaitOptions{
			Target:    p.Pane,
			Until:     until,
			TimeoutMs: smartZonePollMs,
		})
		if err == nil {
			return agent, false, nil
		}
		if !isPollTimeout(err) {
			return agent, false, err
		}
		elapsedMs += smartZonePollMs

		if haveBaseline && elapsedMs >= smartZoneCompactTimeoutMs {
			count, ok, readErr := sessionCompactions(d, p.Agent, p.SessionCwd, sessionID)
			if readErr == nil && ok && count > baselineCompactions {
				return agent, true, nil
			}
		}

		if elapsedMs >= smartZoneCompactExtendedTimeoutMs {
			return agent, false, err
		}
	}
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
	p.sink().IterationPaused(p.Label, PauseRateLimit, reason)
	p.logAgentEvent(eventPausedRateLimit, sessionID, reason)
	waitForCodexRateLimitReset(d, p.SessionCwd, sessionID, limit)
	p.Gate.ForceResume(p.Label)
	p.sink().IterationResumed(p.Label, PauseRateLimit)
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

func codexRateLimit(d Deps, cwd, sessionID, pane string) (codexsession.RateLimit, bool, error) {
	if sessionID != "" && d.ReadCodexRateLimit != nil {
		limit, exhausted, err := d.ReadCodexRateLimit(cwd, sessionID)
		if err != nil || exhausted {
			return limit, exhausted, err
		}
	}
	if d.ReadPaneRecent == nil {
		return codexsession.RateLimit{}, false, nil
	}
	text, err := d.ReadPaneRecent(pane)
	if err != nil {
		return codexsession.RateLimit{}, false, err
	}
	now := time.Now()
	if d.Now != nil {
		now = d.Now()
	}
	limit, matched := detectCodexRateLimit(text, now)
	return limit, matched, nil
}

// codexQuotaOrContextExhaustion classifies a single ReadPaneRecent snapshot
// as a quota exhaustion or a context-window exhaustion, reading the pane at
// most once — unlike calling codexRateLimit and detectCodexContextExhaustion
// separately, which would each read the pane independently and risk missing
// a transient banner that clears between the two reads.
func codexQuotaOrContextExhaustion(d Deps, cwd, sessionID, pane string) (limit codexsession.RateLimit, exhausted bool, evidence string, err error) {
	if sessionID != "" && d.ReadCodexRateLimit != nil {
		limit, exhausted, err = d.ReadCodexRateLimit(cwd, sessionID)
		if err != nil || exhausted {
			return limit, exhausted, "", err
		}
	}
	if d.ReadPaneRecent == nil {
		return codexsession.RateLimit{}, false, "", nil
	}
	text, err := d.ReadPaneRecent(pane)
	if err != nil {
		return codexsession.RateLimit{}, false, "", err
	}
	now := time.Now()
	if d.Now != nil {
		now = d.Now()
	}
	if quotaLimit, matched := detectCodexRateLimit(text, now); matched {
		return quotaLimit, true, "", nil
	}
	evidence, _ = detectCodexContextExhaustion(text)
	return codexsession.RateLimit{}, false, evidence, nil
}

func detectCodexContextExhaustion(text string) (string, bool) {
	for _, line := range strings.Split(text, "\n") {
		evidence := strings.TrimSpace(line)
		lower := strings.ToLower(evidence)
		structuredCode := strings.Contains(lower, `"code"`) && strings.Contains(lower, "context_length_exceeded")
		streamFailure := strings.Contains(lower, "stream disconnected before completion") &&
			strings.Contains(lower, "your input exceeds the context window of this model")
		terminalFailure := strings.HasPrefix(strings.TrimLeft(lower, "■⚠️ \t"), "codex ran out of room in the model's context window")
		if structuredCode || streamFailure || terminalFailure {
			return evidence, true
		}
	}
	return "", false
}

func recoverCodexContextExhaustion(d Deps, p launchAndPromptParams, sessionID string, smartZone int) (bool, error) {
	if d.ReadPaneRecent == nil {
		return false, nil
	}
	text, err := d.ReadPaneRecent(p.Pane)
	if err != nil {
		return false, nil
	}
	evidence, exhausted := detectCodexContextExhaustion(text)
	if !exhausted {
		return false, nil
	}
	if err := d.AgentSendKeys(p.Pane, "ctrl+c"); err != nil {
		return false, fmt.Errorf("interrupting %s after Codex context exhaustion: %w", p.Label, err)
	}
	if err := recoverOrFailCodexContextExhaustion(d, p, sessionID, evidence, smartZone); err != nil {
		return false, err
	}
	return true, nil
}

// recoverOrFailCodexContextExhaustion runs the compact/finish-up contract for
// a classified native Codex context exhaustion and turns an incomplete
// recovery into a durable, actionable error instead of letting the caller
// fall back into ordinary polling. Without this, a recovery whose /compact or
// finish-up prompt never lands (the agent's context is already too far gone
// to process either) leaves the pane sitting idle post-interrupt with no
// further evidence of what happened — waitForFinish would then read that as
// a plain finish, and finishIteration would mark it done (if a stray commit
// happened to land) or generic needs-info (if not), losing the exhaustion
// reason entirely. Returning an error here instead routes through the same
// path every other iteration-level failure takes (see Run's per-result
// handling in loop.go), which marks the ticket needs-attention with this
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

// isPollTimeout reports whether err looks like AgentWait's own
// timeout-elapsed failure (herdr's "timed out waiting for agent status"),
// as opposed to a genuine failure that should abort the loop instead of
// looping back for another poll tick.
func isPollTimeout(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "timed out")
}
