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
// caller goes on to wait for the iteration to finish anyway; blocked goes
// through parkOnBlockedPane and returns errBlockedPaneParked when it parks.
// Any other state, or a block that cleared before the re-check, returns the
// original error.
func promptIdle(d Deps, p launchAndPromptParams, sessionID, text string) error {
	err := d.Runner.Prompt(p.session(), text)
	if !errors.Is(err, agentrunner.ErrNotReady) {
		return err
	}
	st, statusErr := d.Runner.Status(p.session())
	if statusErr != nil {
		return fmt.Errorf("re-reading %s after refused prompt: %w", p.Label, statusErr)
	}
	switch st.State {
	case agentrunner.StateWorking:
		return nil
	case agentrunner.StateBlocked:
		parked, parkErr := parkOnBlockedPane(d, p, sessionID)
		if parkErr != nil {
			return parkErr
		}
		if parked {
			return errBlockedPaneParked
		}
	}
	return err
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
	if st.BlockedReason != "" {
		reason = fmt.Sprintf("%s is blocked on a prompt gx did not send (%s); answer it with `gx server agents answer`, or in the pane", p.Label, st.BlockedReason)
	}
	p.parkBlockedPane(sessionID, reason)
	return true, nil
}

// parkBlockedPane routes a blocked-pane park through the single park path,
// keeping the agent context on the event.
func (p launchAndPromptParams) parkBlockedPane(sessionID, reason string) {
	park(p.sink(), parkRequest{
		ScratchDir: p.ScratchDir, EpicName: p.EpicName, Ticket: p.Ticket, Path: p.TicketPath,
		Type: events.NeedsAnswer, Kind: events.BlockedPane, Reason: reason,
		Event: Event{Agent: p.Agent, Pane: p.Pane, AgentSession: sessionID, Cwd: p.SessionCwd},
	})
}
