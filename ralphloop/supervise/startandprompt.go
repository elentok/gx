package supervise

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/deps"
	"github.com/elentok/gx/ralphloop/runlog"
)

// StallNudgeText is the fixed text of the stall nudge.
const StallNudgeText = "If you are waiting for a person's reply, run `gx tickets ask` now. Otherwise continue."

// LaunchFailure carries the events kind of a failed launch up to the loop's
// catch-all pause, so the pause event matches the launch-failed events.
type LaunchFailure struct {
	Kind events.Kind
	Err  error
}

func (e *LaunchFailure) Error() string { return e.Err.Error() }
func (e *LaunchFailure) Unwrap() error { return e.Err }

// LaunchFailureKind is the events kind of a launch failure in err's chain,
// for a caller that pauses the ticket itself.
func LaunchFailureKind(err error) (events.Kind, bool) {
	var lf *LaunchFailure
	if errors.As(err, &lf) {
		return lf.Kind, true
	}
	return "", false
}

// StartAndPromptLogged starts a session and sends its initial prompt. A prompt the
// agent never picks up (ErrNotDelivered) points at a bad session rather than a
// slow agent, so it gets exactly one fresh session before pausing as
// agent_prompt_stalled. Any other prompt error (e.g. ErrNotReady on a blocked
// agent) returns with the session still live, for the blocked-pane pause.
// onFail is called for every failed attempt, to log launch-failed, and
// onNudge for every nudge decision the runner made on the prompt.
//
// A label already live before Start means Start adopted our own earlier
// session (e.g. a second gx process racing the same ticket): that one is
// returned unprompted with Adopted set, since it may be mid-turn.
func StartAndPromptLogged(ctx context.Context, r agentrunner.Runner, opts agentrunner.StartOptions, prompt string, onFail func(attempt int, kind events.Kind, err error), onNudge func(attempt int, decision string)) (Launched, error) {
	return startAndPrompt(ctx, r, opts, prompt, onFail, onNudge, nil)
}

// startAndPrompt is StartAndPromptLogged with onStarted, called on every
// attempt once Start returns and before the prompt is sent.
func startAndPrompt(ctx context.Context, r agentrunner.Runner, opts agentrunner.StartOptions, prompt string, onFail func(attempt int, kind events.Kind, err error), onNudge func(attempt int, decision string), onStarted func(attempt int)) (Launched, error) {
	_, adopted, err := r.Find(opts.Label)
	if err != nil {
		// Not fatal: Start still runs, it just can't be an adoption.
		log.Printf("looking up %s before start: %v", opts.Label, err)
	}
	for attempt := 1; ; attempt++ {
		s, err := r.Start(opts)
		if err != nil {
			kind := events.IterationError
			if errors.Is(err, agentrunner.ErrLabelTaken) {
				kind = events.AgentNameTaken
			}
			err = withDoctorHint(fmt.Errorf("starting %s: %w", opts.Label, err))
			onFail(attempt, kind, err)
			return Launched{}, &LaunchFailure{Kind: kind, Err: err}
		}
		if adopted {
			return Launched{Session: s, Adopted: true}, nil
		}
		if onStarted != nil {
			onStarted(attempt)
		}
		status, err := r.Status(s)
		if err != nil {
			log.Printf("reading %s status before initial prompt: %v", opts.Label, err)
		}
		err = agentrunner.PromptLaunch(ctx, r, s, prompt, func(decision string) { onNudge(attempt, decision) })
		if !errors.Is(err, agentrunner.ErrNotDelivered) {
			return Launched{Session: s, Baseline: status.Turn}, withDoctorHint(err)
		}
		err = fmt.Errorf("sending initial prompt: %w", err)
		onFail(attempt, events.AgentPromptStalled, err)
		if stopErr := r.Stop(s); stopErr != nil {
			log.Printf("stopping undelivered session %s: %v", s.Label, stopErr)
		}
		if attempt == 2 {
			return Launched{}, &LaunchFailure{Kind: events.AgentPromptStalled, Err: withSkillHint(err, prompt)}
		}
	}
}

// withSkillHint names the missing skill and the fix on an unknown-skill
// failure: a missing skill does not appear by itself, so a bare Retry would hit
// the same error. Any other error passes through unchanged.
func withSkillHint(err error, prompt string) error {
	if !errors.Is(err, agentrunner.ErrUnknownSkill) {
		return err
	}
	skill, _, _ := strings.Cut(prompt, " ")
	return fmt.Errorf("the agent does not know the skill %s: install or refresh it with `gx skills install`, then Retry: %w", skill, err)
}

// withDoctorHint points a missing-capability error at the command that
// diagnoses it; any other error, nil included, passes through unchanged.
func withDoctorHint(err error) error {
	if errors.Is(err, agentrunner.ErrMissingCapability) {
		return fmt.Errorf("%w (run `gx claude doctor`)", err)
	}
	return err
}

// StartAndPrompt is StartAndPromptLogged for callers outside the loop (the server's
// launch): same retry on an undelivered prompt, no launch-failed events. It
// never adopts a holder of the label (ADR 0039). onStarted may be nil.
func StartAndPrompt(ctx context.Context, r agentrunner.Runner, opts agentrunner.StartOptions, prompt string, onNudge func(attempt int, decision string), onStarted func(attempt int)) (agentrunner.Session, error) {
	opts.NoAdopt = true
	l, err := startAndPrompt(ctx, r, opts, prompt, func(int, events.Kind, error) {}, onNudge, onStarted)
	return l.Session, err
}

// Launched is a session StartAndPromptLogged started and prompted, or adopted.
type Launched struct {
	agentrunner.Session
	Adopted bool
	// Baseline is the session's Turn before its initial prompt: the
	// stalled-since-launch baseline (see noActivitySinceLaunch). Zero when
	// adopted or unknown.
	Baseline int
}

// PromptedLaunch is the post-prompt half of a launch for a session started
// and prompted through Deps.Runner, with baseline its pre-prompt Turn.
func PromptedLaunch(d deps.Deps, p LaunchParams, baseline int) (string, error) {
	status, err := d.Runner.Status(p.Session)
	if err != nil {
		log.Printf("reading %s status after initial prompt: %v", p.Label, err)
	}
	// Codex only knows its session id once the first prompt begins.
	sessionID := status.SessionID
	if sessionID == "" {
		sessionID = p.Session.SessionID
	}
	return startedLaunch(d, p, sessionID, baseline)
}

// startedLaunch logs and reports a freshly prompted agent's start, with seq as
// its stalled-since-launch baseline, then waits for it to finish.
func startedLaunch(d deps.Deps, p LaunchParams, sessionID string, seq int) (string, error) {
	reportStarted(d, p, sessionID, seq)
	return waitLaunched(d, p, sessionID)
}

func reportStarted(d deps.Deps, p LaunchParams, sessionID string, seq int) {
	p.LogAgentStartEvent(p.StartEvent, sessionID, seq)
	if p.StartEvent != "" {
		p.EventSink().IterationStarted(p.TicketData, p.Label, p.SessionCwd, sessionID, p.Agent, p.Pane)
		EmitContextOccupancy(d, p.EventSink(), p.Agent, p.Ticket, p.SessionCwd, sessionID)
	}
}

// AdoptedLaunch takes over p.Session, a live session StartAndPromptLogged adopted
// unprompted. An idle one whose turn hasn't moved past its logged launch
// baseline (see noActivitySinceLaunch) never got its prompt, so it gets it
// now; an idle one that has moved is already finished; anything else is
// waited out.
func AdoptedLaunch(d deps.Deps, p LaunchParams) (string, error) {
	status, err := d.Runner.Status(p.Session)
	if err != nil {
		return "", fmt.Errorf("reading adopted %s status: %w", p.Label, err)
	}
	sessionID := cmp.Or(status.SessionID, p.Session.SessionID)
	// Seq 0 keeps the original launch's baseline the one noActivitySinceLaunch
	// finds.
	reportStarted(d, p, sessionID, 0)
	if AlreadyFinished(string(status.State)) {
		if !noActivitySinceLaunch(p.ScratchDir, p.EpicName, sessionID, status.Turn) {
			p.LogLifecycleEvent(p.FinishEvent, sessionID)
			return sessionID, nil
		}
		if err := PromptIdle(d, p, p.Prompt); err != nil {
			return "", fmt.Errorf("sending initial prompt to stalled adopted session: %w", err)
		}
	}
	return waitLaunched(d, p, sessionID)
}

// waitLaunched is WaitForFinish keeping sessionID on a blocked-pane pause,
// whose caller still records the live session.
func waitLaunched(d deps.Deps, p LaunchParams, sessionID string) (string, error) {
	if err := WaitForFinish(d, p, sessionID); err != nil {
		if errors.Is(err, ErrBlockedPanePaused) {
			return sessionID, err
		}
		return "", err
	}
	return sessionID, nil
}

// noActivitySinceLaunch reports whether a pane's current state_change_seq
// still matches the baseline stamped on its own iteration-started event (see
// LogAgentStartEvent), found in the epic's run log by matching agentSession.
// AdoptedLaunch uses it as its "has this session done anything since this
// iteration launched it" check. A missing log, an untracked session, or no
// matching event all fall through to false — the safe default of trusting the
// caller's existing behavior.
func noActivitySinceLaunch(scratchDir, epicName, agentSession string, currentSeq int) bool {
	if agentSession == "" {
		return false
	}
	evs, ok, err := runlog.ReadEvents(scratchDir, epicName)
	if !ok || err != nil {
		return false
	}
	for _, ev := range evs {
		if ev.Type == string(events.IterationStarted) && ev.AgentSession == agentSession && ev.StateChangeSeq != 0 {
			return currentSeq == ev.StateChangeSeq
		}
	}
	return false
}
