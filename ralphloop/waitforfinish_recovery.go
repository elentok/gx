package ralphloop

import (
	"errors"
	"fmt"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
)

// recoverCodexContextExhaustionPoll handles Codex context-exhaustion evidence
// seen during one waitForFinish poll and returns the poll's new elapsed time.
// The caller always continues its loop afterwards.
//
// paceRetry says whether the poll that saw the evidence returned at once
// (a blocked or idle pane: Wait does not block there), in which case a retry
// sleeps one poll interval and counts it. A timed-out Wait already spent its
// interval, so it passes false.
func recoverCodexContextExhaustionPoll(d Deps, p launchAndPromptParams, sessionID, evidence string, smartZone, elapsedMs int, paceRetry bool) (int, error) {
	retry, err := interruptOrRetry(d, p, "Codex context exhaustion")
	if err != nil {
		return elapsedMs, err
	}
	if retry {
		if paceRetry {
			d.Sleep(smartZonePollMs * time.Millisecond)
			return elapsedMs + smartZonePollMs, nil
		}
		return elapsedMs, nil
	}
	if err := recoverOrFailCodexContextExhaustion(d, p, sessionID, evidence, smartZone); err != nil {
		return elapsedMs, err
	}
	return 0, nil
}

// promptIdle sends text to a session its caller expects to be idle. A
// refusal with ErrNotReady means that expectation was stale, so Status is
// re-read: busy (the agent is already working) skips the prompt, since the
// caller goes on to wait for the iteration to finish anyway. Any other state
// returns the original error.
func promptIdle(d Deps, p launchAndPromptParams, text string) error {
	err := d.Runner.Prompt(p.session(), text)
	if !errors.Is(err, agentrunner.ErrNotReady) {
		return err
	}
	st, statusErr := d.Runner.Status(p.session())
	if statusErr != nil {
		return fmt.Errorf("re-reading %s after refused prompt: %w", p.Label, statusErr)
	}
	if st.State == agentrunner.StateWorking {
		return nil
	}
	return err
}

// blockedDwellMs is the fixed window the server wait loop waits, once, after first
// observing a pane blocked, before dwellThenPauseBlocked's single re-check. A
// momentary block (the agent clearing its own transient prompt) shouldn't
// pause a healthy iteration, but this is a fixed window, not a settle timer:
// a pane that leaves and re-enters the blocked state inside it does not
// restart the wait, and nothing about the pane is observed again until the
// window ends.
const blockedDwellMs = 15_000

// errBlockedPanePaused is the sentinel return for a pane paused on an
// unanswered prompt: not a failure, but a signal to the callers (launchAndPrompt, runIteration, reattachIteration) to
// end the iteration without running the ordinary finish path (commit check,
// cherry-pick, worktree/tab cleanup) that assumes the agent actually finished
// — this pane's agent is still live, just waiting on a prompt nobody has
// answered yet.
var errBlockedPanePaused = errors.New("iteration paused: pane blocked on an unanswered prompt")

// dwellThenPauseBlocked is the server wait loop's dwell-and-recheck
// (WaitIterationFinished): it waits out blockedDwellMs, then re-reads the
// session once via Runner.Status (a peek, not another Wait) and, only if it is
// still blocked at that instant, writes the pane-answered pause (paused
// (answer), a reason, and a "## Needs Answer" stub, both naming p.Label — the
// pane is named by its iteration label, never a raw pane id, since a label
// still resolves after a restart or reattach and a pane id does not).
// runner names the runner whose block this is, so the reason names the ways to
// answer on it.
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
func dwellThenPauseBlocked(d Deps, p launchAndPromptParams, runner, sessionID string) (reason string, paused bool, err error) {
	d.Sleep(blockedDwellMs * time.Millisecond)

	st, err := d.Runner.Status(p.session())
	if err != nil {
		return "", false, fmt.Errorf("rechecking %s for pause: %w", p.Label, err)
	}
	if st.State != agentrunner.StateBlocked {
		return "", false, nil
	}

	how := "answer it in the pane"
	if st.BlockedReason != "" {
		how = "answer it with `gx server agents answer`, or in the pane"
	}
	if runner == string(agentrunner.ChoicePTY) {
		how = "answer it with the web answer button, `gx server agents answer`, or by typing in the Agents tab live panel"
	}
	reason = fmt.Sprintf("%s is blocked on a prompt gx did not send; %s", p.Label, how)
	if st.BlockedReason != "" {
		reason = fmt.Sprintf("%s is blocked on a prompt gx did not send (%s); %s", p.Label, st.BlockedReason, how)
	}
	p.pauseBlockedPane(sessionID, reason)
	return reason, true, nil
}

// pauseBlockedAtOnce is dwellThenPauseBlocked without the dwell, for a headless
// agent whose block st the tracker reported. The reason names the tool and its
// input, with the verb that exits that block: a permission request is answered,
// a bare requires_action (no request to answer) is interrupted.
func pauseBlockedAtOnce(d Deps, p launchAndPromptParams, ticketPath string, st agentrunner.Status) error {
	hint := "interrupt with `gx server agents interrupt`"
	if st.Pending != nil {
		hint = "answer with `gx server agents answer`"
	}
	reason := fmt.Sprintf("%s is blocked: %s; %s", p.Label, st.BlockedReason, hint)
	p.pauseBlockedPane(recordLiveSession(d, p.Label, ticketPath), reason)
	return &BlockedPausedError{Reason: reason}
}

// pauseBlockedPane routes a blocked-pane pause through the single pause path,
// keeping the agent context on the event.
func (p launchAndPromptParams) pauseBlockedPane(sessionID, reason string) {
	pause(p.sink(), pauseRequest{
		ScratchDir: p.ScratchDir, EpicName: p.EpicName, Ticket: p.Ticket, Path: p.TicketPath,
		Type: events.Paused, Kind: events.BlockedPane, Reason: reason,
		Event: Event{Agent: p.Agent, Pane: p.Pane, AgentSession: sessionID, Cwd: p.SessionCwd},
	})
}
