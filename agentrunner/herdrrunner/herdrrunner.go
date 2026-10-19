// Package herdrrunner implements agentrunner.Runner on herdr: one workspace
// per epic, one tab and agent per session, with the agent named after the
// session label. Session.ID is the agent's pane id.
package herdrrunner

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/herdr"
)

// pollInterval spaces AgentWait retries when herdr answers a wait with a
// timeout before the caller's own deadline.
const pollInterval = 20 * time.Millisecond

// backgroundTaskPollInterval spaces Wait's rounds while a background task
// overrides herdr's idle: herdr answers each wait at once then, so the pace is
// all that stops a busy loop of herdr spawns and transcript reads.
const backgroundTaskPollInterval = 2 * time.Second

type Runner struct {
	// BackgroundTasks reports whether the agent behind s still has a
	// background task running. herdr shows such an agent idle, but the task
	// will wake it, so the runner reports it working. Nil means no gating.
	BackgroundTasks func(s agentrunner.Session) bool
	// CodexQuota is checked before the pane for Codex sessions. Nil skips it.
	CodexQuota CodexQuotaReader
	// Now and Sleep pace Wait's polling. Nil means the wall clock; tests on
	// herdrfake swap in its virtual one (Coordinator.Clock).
	Now   func() time.Time
	Sleep func(time.Duration)
	// N is the configured runner wait; zero means agentrunner.DefaultN.
	N time.Duration

	mu sync.Mutex
	// started remembers what RateLimit needs from Start, keyed by label.
	started map[string]agentrunner.StartOptions
	// held marks the sessions (by pane id) in the middle of a deny with a
	// reason: Status reports them blocked and the other verbs refuse.
	held map[string]bool
}

var _ agentrunner.Runner = (*Runner)(nil)
var _ agentrunner.HealthChecker = (*Runner)(nil)
var _ agentrunner.Nudger = (*Runner)(nil)

// New gates on Claude transcript background tasks, unless the home directory
// can't be found.
func New() *Runner {
	r := &Runner{started: map[string]agentrunner.StartOptions{}, CodexQuota: codexsession.LastRateLimit}
	if home, err := os.UserHomeDir(); err == nil {
		r.BackgroundTasks = claudeui.BackgroundTasks(home, time.Now)
	}
	return r
}

// WaitN is N, the configured runner wait.
func (r *Runner) WaitN() time.Duration {
	if r.N <= 0 {
		return agentrunner.DefaultN
	}
	return r.N
}

func (r *Runner) Healthy() error { return herdr.Ping() }

func (r *Runner) Start(opts agentrunner.StartOptions) (agentrunner.Session, error) {
	wsID, err := herdr.EnsureWorkspace(opts.Epic, opts.Cwd)
	if err != nil {
		return agentrunner.Session{}, err
	}
	tab, err := herdr.TabCreate(herdr.TabCreateOptions{WorkspaceID: wsID, Cwd: opts.Cwd, Label: opts.Label, Env: opts.Env})
	if err != nil {
		return agentrunner.Session{}, err
	}
	agent, err := herdr.AgentStart(herdr.AgentStartOptions{Name: opts.Label, Kind: string(opts.Kind), Pane: tab.RootPaneID, AgentArgs: opts.Args})
	adopted := false
	if err != nil {
		recovery, err := RecoverStart(err, opts.Label, opts.Cwd, tab.RootPaneID, !opts.NoAdopt, herdr.AgentExplain, herdr.AgentSendKeys)
		if recovery != StartResume {
			_ = herdr.TabClose(tab.TabID)
		}
		switch {
		case err != nil:
			return agentrunner.Session{}, err
		case recovery == StartAdopt:
			if agent, err = herdr.AgentGet(opts.Label); err != nil {
				return agentrunner.Session{}, err
			}
			adopted = true
		default:
			agent.PaneID = tab.RootPaneID
		}
	}
	s := agentrunner.Session{Label: opts.Label, ID: agent.PaneID, SessionID: agent.AgentSession}
	r.mu.Lock()
	r.started[opts.Label] = opts
	r.mu.Unlock()
	if adopted {
		return s, nil
	}
	if _, err := r.Wait(s, []agentrunner.State{agentrunner.StateIdle, agentrunner.StateDone}, 30*time.Second); err != nil {
		return agentrunner.Session{}, err
	}
	return s, nil
}

func (r *Runner) Prompt(s agentrunner.Session, text string) error {
	return r.PromptNudged(s, text, nil)
}

func (r *Runner) PromptNudged(s agentrunner.Session, text string, onNudge func(string)) error {
	return r.promptNudged(s, text, onNudge, false)
}

// promptNudged is PromptNudged; ownHold lets the deny's own reason prompt
// through the hold that refuses everyone else.
func (r *Runner) promptNudged(s agentrunner.Session, text string, onNudge func(string), ownHold bool) error {
	if !ownHold && r.isHeld(s) {
		return fmt.Errorf("%w: blocked on a deny with a reason", agentrunner.ErrNotReady)
	}
	turn, err := r.awaitNotBusy(s)
	if err != nil {
		return err
	}
	if isSlashCommand(text) {
		return r.promptSlash(s, text, turn)
	}
	compact := text == "/compact"
	until := []string{"working"}
	if compact {
		// Codex asks to confirm a compact before it starts. A premature-idle
		// Claude pane reports idle/done while still compacting and never
		// "working"; accepting those keeps promptWithNudge from Enter-nudging it
		// (which could cancel the compaction) and leaves the transcript gate to
		// decide when compaction has really finished.
		until = append(until, "blocked", "idle", "done")
	}
	// A session this runner did not start is read as Claude, as in RateLimit.
	r.mu.Lock()
	kind := r.started[s.Label].Kind
	r.mu.Unlock()
	if kind == "" {
		kind = harness.Claude
	}
	typing, err := harness.LookupTyping(kind)
	if err != nil {
		return err
	}
	// A turn can start and end between herdr's polls, so its wait reports a
	// stall though the seq moved: that turn was delivered, and a nudge or
	// retype would send the text twice.
	submit := func(o herdr.AgentPromptOptions) (herdr.Agent, error) {
		agent, err := herdr.AgentPrompt(o)
		if IsPollTimeout(err) {
			if cur, getErr := herdr.AgentGet(s.ID); getErr == nil && cur.StateChangeSeq > turn {
				return cur, errTurnPassedUnseen
			}
		}
		return agent, err
	}
	prompt := promptWithNudge(typing, submit, herdr.AgentSendKeys, herdr.AgentWait, herdr.AgentRead, time.Now, claudeui.ReportDecisions(onNudge))
	agent, err := prompt(herdr.AgentPromptOptions{Target: s.ID, Text: text, Wait: true, Until: until})
	var blocked *herdr.AgentBlockedError
	switch {
	case errors.Is(err, errTurnPassedUnseen):
		return nil
	case errors.As(err, &blocked):
		return fmt.Errorf("%w: %s", agentrunner.ErrNotReady, blocked.Message)
	case errors.Is(err, claudeui.ErrStuckSubmission):
		return fmt.Errorf("%w: %w", agentrunner.ErrNotDelivered, err)
	case err != nil:
		return err
	}
	if agent.StateChangeSeq <= turn {
		return agentrunner.ErrNotDelivered
	}
	if compact {
		return r.confirmCompact(s, agent.AgentStatus)
	}
	return nil
}

var errTurnPassedUnseen = errors.New("turn started and ended between polls")

// isSlashCommand reports whether text is a "/" command that takes the
// single-send rule. /compact keeps the normal path: it starts a turn.
func isSlashCommand(text string) bool {
	return strings.HasPrefix(text, "/") && text != "/compact"
}

// awaitNotBusy waits up to N for herdr to stop showing the agent working and
// returns the Turn to send against. A prompt typed into a working agent is
// only queued, so its next idle edge would pass for delivery. herdr's state is
// read before the background-task probe: a background task alone is not busy.
func (r *Runner) awaitNotBusy(s agentrunner.Session) (int, error) {
	now, sleep := r.clock()
	deadline := now().Add(r.WaitN())
	for {
		st, err := r.herdrStatus(s)
		if err != nil {
			return 0, err
		}
		switch st.State {
		case agentrunner.StateBlocked:
			return 0, fmt.Errorf("%w: blocked on %s", agentrunner.ErrNotReady, st.BlockedReason)
		case agentrunner.StateWorking:
			if !now().Before(deadline) {
				return 0, fmt.Errorf("%w: busy for %s", agentrunner.ErrNotReady, r.WaitN())
			}
			sleep(min(pollInterval, deadline.Sub(now())))
		default:
			return st.Turn, nil
		}
	}
}

// promptSlash sends a "/" command once, never nudging or retyping: most
// commands start no turn, so a retype would run them twice. Delivered means
// the seq moved (a turn, a new conversation, a dialog) or herdr still shows
// the agent idle after N.
func (r *Runner) promptSlash(s agentrunner.Session, text string, turn int) error {
	if _, err := herdr.AgentPrompt(herdr.AgentPromptOptions{Target: s.ID, Text: text}); err != nil {
		return mapNotFound(err)
	}
	now, sleep := r.clock()
	deadline := now().Add(r.WaitN())
	for {
		st, err := r.herdrStatus(s)
		if err != nil {
			return err
		}
		if st.Turn > turn {
			return nil
		}
		if !now().Before(deadline) {
			if st.State == agentrunner.StateWorking {
				return fmt.Errorf("%w: %q started no turn and the agent is not idle", agentrunner.ErrNotDelivered, text)
			}
			return nil
		}
		sleep(min(pollInterval, deadline.Sub(now())))
	}
}

// clock is Now and Sleep, defaulting to the wall clock.
func (r *Runner) clock() (func() time.Time, func(time.Duration)) {
	now, sleep := time.Now, time.Sleep
	if r.Now != nil {
		now = r.Now
	}
	if r.Sleep != nil {
		sleep = r.Sleep
	}
	return now, sleep
}

// Compact timings are vars so tests can shorten them.
var (
	compactConfirmTimeout = 5 * time.Minute
	compactSubmitPoll     = 5 * time.Second
	compactSubmitTimeout  = 30 * time.Second
)

// confirmCompact passively waits out Codex's compact confirmation (it moves
// on by itself), then waits for the pane's last line to stop reading
// "/compact": herdr can see the state change before Enter's effect renders,
// and a prompt sent then lands appended to the unsubmitted "/compact". It
// never presses Enter itself, which could cancel a running compaction.
func (r *Runner) confirmCompact(s agentrunner.Session, state string) error {
	if state == string(agentrunner.StateBlocked) {
		running := []agentrunner.State{agentrunner.StateWorking, agentrunner.StateIdle, agentrunner.StateDone}
		if _, err := r.Wait(s, running, compactConfirmTimeout); err != nil {
			return fmt.Errorf("%w: waiting out the compact confirmation: %w", agentrunner.ErrNotDelivered, err)
		}
	}
	deadline := time.Now().Add(compactSubmitTimeout)
	for {
		text, err := ReadPaneRecent(s.ID)
		if err != nil {
			return mapNotFound(err)
		}
		lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
		if strings.TrimSpace(lines[len(lines)-1]) != "/compact" {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w: /compact still unsubmitted in the pane", agentrunner.ErrNotDelivered)
		}
		time.Sleep(compactSubmitPoll)
	}
}

func (r *Runner) Status(s agentrunner.Session) (agentrunner.Status, error) {
	return r.status(s)
}

func (r *Runner) Wait(s agentrunner.Session, states []agentrunner.State, timeout time.Duration) (agentrunner.Status, error) {
	until := make([]string, len(states))
	for i, st := range states {
		until[i] = string(st)
	}
	now, sleep := r.clock()
	deadline := now().Add(timeout)
	for {
		remaining := deadline.Sub(now())
		_, err := herdr.AgentWait(herdr.AgentWaitOptions{Target: s.ID, Until: until, TimeoutMs: max(1, int(remaining.Milliseconds()))})
		if err != nil && !IsPollTimeout(err) {
			return agentrunner.Status{}, mapNotFound(err)
		}
		pause := pollInterval
		if err == nil {
			// herdr's state can still be overridden by a background task.
			st, err := r.status(s)
			if err != nil || slices.Contains(states, st.State) {
				return st, err
			}
			pause = backgroundTaskPollInterval
		}
		if !now().Before(deadline) {
			st, err := r.status(s)
			if err != nil {
				return agentrunner.Status{}, err
			}
			return st, agentrunner.ErrTimeout
		}
		sleep(min(pause, deadline.Sub(now())))
	}
}

// Interrupt picks its key by herdr's state, read right before the key: Ctrl+C
// when working, Esc when blocked (it denies the dialog), nothing otherwise
// (including a state gx doesn't know). After a key it waits up to N for idle or
// done, then clears the input line: claude restores the cancelled prompt into
// it, and it would prefix the next prompt. Claude gets Ctrl+C like the PTY
// runner, since Ctrl+U clears one line per press and leaves a multi-line prompt
// behind. Codex keeps Ctrl+U: Ctrl+C on its empty composer quits the agent.
func (r *Runner) Interrupt(s agentrunner.Session) error {
	if r.isHeld(s) {
		return fmt.Errorf("%w: a deny with a reason is in progress", agentrunner.ErrNotReady)
	}
	agent, err := herdr.AgentGet(s.ID)
	if err != nil {
		return mapNotFound(err)
	}
	var key string
	switch agentrunner.State(agent.AgentStatus) {
	case agentrunner.StateWorking:
		key = "ctrl+c"
	case agentrunner.StateBlocked:
		key = "esc"
	default:
		return nil
	}
	if err := herdr.AgentSendKeys(s.ID, key); err != nil {
		return mapNotFound(err)
	}
	if err := r.awaitIdle(s); err != nil {
		return err
	}
	r.mu.Lock()
	kind := r.started[s.Label].Kind
	r.mu.Unlock()
	clear := "ctrl+c"
	if kind == "codex" {
		clear = "ctrl+u"
	}
	return mapNotFound(herdr.AgentSendKeys(s.ID, clear))
}

// awaitIdle waits up to N for herdr to show the agent idle or done.
func (r *Runner) awaitIdle(s agentrunner.Session) error {
	now, sleep := r.clock()
	deadline := now().Add(r.WaitN())
	for {
		remaining := deadline.Sub(now())
		_, err := herdr.AgentWait(herdr.AgentWaitOptions{Target: s.ID, Until: []string{"idle", "done"}, TimeoutMs: max(1, int(remaining.Milliseconds()))})
		if err == nil {
			return nil
		}
		if !IsPollTimeout(err) {
			return mapNotFound(err)
		}
		if !now().Before(deadline) {
			return fmt.Errorf("%w: not idle after %s", agentrunner.ErrNotReady, r.WaitN())
		}
		sleep(min(pollInterval, deadline.Sub(now())))
	}
}

func (r *Runner) Stop(s agentrunner.Session) error {
	agent, err := herdr.AgentGet(s.ID)
	if isNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := herdr.TabClose(agent.TabID); err != nil {
		return err
	}
	r.mu.Lock()
	delete(r.started, s.Label)
	r.mu.Unlock()
	return nil
}

// Find looks the agent up by name, since only agents (not tabs) are named
// after the session label for every launch path.
func (r *Runner) Find(label string) (agentrunner.Session, bool, error) {
	agent, err := herdr.AgentGet(label)
	if isNotFound(err) {
		return agentrunner.Session{}, false, nil
	}
	if err != nil {
		return agentrunner.Session{}, false, err
	}
	return agentrunner.Session{Label: label, ID: agent.PaneID, SessionID: agent.AgentSession}, true, nil
}

// List returns the epic workspace's named agents. Tab labels are not agent
// names: ralph-loop's server launch labels a tab after its ticket.
func (r *Runner) List(epic string) ([]agentrunner.Session, error) {
	wsID, err := herdr.FindWorkspace(epic)
	if err != nil || wsID == "" {
		return nil, err
	}
	agents, err := herdr.AgentList()
	if err != nil {
		return nil, err
	}
	var out []agentrunner.Session
	for _, agent := range agents {
		if agent.WorkspaceID == wsID && agent.Name != "" {
			out = append(out, agentrunner.Session{Label: agent.Name, ID: agent.PaneID, SessionID: agent.AgentSession})
		}
	}
	return out, nil
}

// RateLimit scrapes the pane for the agent's limit message. A session this
// runner did not start is read as Claude; Claude's pattern also matches
// Codex's quota line, just without its reset time.
func (r *Runner) RateLimit(s agentrunner.Session) (time.Time, bool, error) {
	r.mu.Lock()
	opts := r.started[s.Label]
	r.mu.Unlock()
	now := time.Now()
	if opts.Kind == "codex" {
		return r.codexRateLimit(s, opts.Cwd, now)
	}
	text, err := ReadPaneRecent(s.ID)
	if err != nil {
		return time.Time{}, false, mapNotFound(err)
	}
	token, limited := claudeui.DetectRateLimit(text)
	if !limited {
		return time.Time{}, false, nil
	}
	if d, ok := claudeui.SecondsUntilReset(token, now); ok {
		return now.Add(d), true, nil
	}
	return time.Time{}, true, nil
}

func (r *Runner) codexRateLimit(s agentrunner.Session, cwd string, now time.Time) (time.Time, bool, error) {
	// s may predate herdr learning the agent's session id.
	if s.SessionID == "" {
		if agent, err := herdr.AgentGet(s.ID); err == nil {
			s.SessionID = agent.AgentSession
		}
	}
	limit, exhausted, evidence, err := CodexQuotaOrContextExhaustion(r.CodexQuota, ReadPaneRecent, now, cwd, s.SessionID, s.ID)
	switch {
	case err != nil:
		return time.Time{}, false, mapNotFound(err)
	case exhausted:
		return limit.ResetAt, true, nil
	case evidence != "":
		return time.Time{}, false, fmt.Errorf("%w: %s", agentrunner.ErrContextExhausted, evidence)
	}
	return time.Time{}, false, nil
}

func (r *Runner) Name() string { return string(agentrunner.ChoiceHerdr) }

func (r *Runner) Answer(s agentrunner.Session, a agentrunner.Answer) error {
	if a.Decision == agentrunner.DecisionDeny && a.Text != "" {
		return r.denyWithReason(s, a.Text)
	}
	if r.isHeld(s) {
		return fmt.Errorf("%w: a deny with a reason is in progress", agentrunner.ErrNotReady)
	}
	if err := r.requireBlocked(s); err != nil {
		return err
	}
	key := "enter"
	if a.Decision == agentrunner.DecisionDeny {
		key = "esc"
	}
	return mapNotFound(herdr.AgentSendKeys(s.ID, key))
}

func (r *Runner) requireBlocked(s agentrunner.Session) error {
	st, err := r.herdrStatus(s)
	if err != nil {
		return err
	}
	if st.State != agentrunner.StateBlocked {
		return agentrunner.ErrNotReady
	}
	return nil
}

// denyWithReason presses Esc (the reason is never typed as dialog keys), waits
// up to N for idle or done, then sends the reason as a prompt. The hold keeps
// every caller seeing blocked from before the Esc until Answer returns, so no
// waiter takes the gap for the turn's end and no other verb lands in it. Once
// Esc is sent, any failure is ErrNotDelivered: the dialog is closed, the
// session alive, the reason not typed.
func (r *Runner) denyWithReason(s agentrunner.Session, reason string) error {
	if !r.hold(s) {
		return fmt.Errorf("%w: a deny with a reason is in progress", agentrunner.ErrNotReady)
	}
	defer r.release(s)
	if err := r.requireBlocked(s); err != nil {
		return err
	}
	if err := herdr.AgentSendKeys(s.ID, "esc"); err != nil {
		return mapNotFound(err)
	}
	err := r.awaitIdle(s)
	if err == nil {
		err = r.promptNudged(s, reason, nil, true)
	}
	switch {
	case err == nil:
		return nil
	case errors.Is(err, agentrunner.ErrNotFound):
		return err
	}
	return fmt.Errorf("%w: denied, but the reason was not delivered: %v", agentrunner.ErrNotDelivered, err)
}

// hold starts s's deny hold, reporting false when one is already running.
func (r *Runner) hold(s agentrunner.Session) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.held[s.ID] {
		return false
	}
	if r.held == nil {
		r.held = map[string]bool{}
	}
	r.held[s.ID] = true
	return true
}

func (r *Runner) release(s agentrunner.Session) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.held, s.ID)
}

func (r *Runner) isHeld(s agentrunner.Session) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.held[s.ID]
}

// status reports herdr's StateChangeSeq as Turn: herdr keeps it, so it holds
// across a gx restart, and it advances on every turn start (and end).
func (r *Runner) status(s agentrunner.Session) (agentrunner.Status, error) {
	st, err := r.herdrStatus(s)
	if err != nil {
		return st, err
	}
	if r.isHeld(s) {
		st.State = agentrunner.StateBlocked
		return st, nil
	}
	if st.State == agentrunner.StateIdle || st.State == agentrunner.StateDone {
		// s may predate herdr learning the agent's session id.
		probed := s
		if st.SessionID != "" {
			probed.SessionID = st.SessionID
		}
		if r.BackgroundTasks != nil && r.BackgroundTasks(probed) {
			st.State = agentrunner.StateWorking
		}
	}
	return st, nil
}

// herdrStatus is herdr's own view of the agent, before any background-task
// override. A state herdr reports that gx doesn't know counts as working.
func (r *Runner) herdrStatus(s agentrunner.Session) (agentrunner.Status, error) {
	agent, err := herdr.AgentGet(s.ID)
	if err != nil {
		return agentrunner.Status{}, mapNotFound(err)
	}
	st := agentrunner.Status{State: agentrunner.State(agent.AgentStatus), Turn: agent.StateChangeSeq, SessionID: agent.AgentSession}
	switch st.State {
	case agentrunner.StateWorking, agentrunner.StateBlocked, agentrunner.StateIdle, agentrunner.StateDone:
	default:
		st.State = agentrunner.StateWorking
	}
	if st.State == agentrunner.StateBlocked {
		st.BlockedReason = MatchedRuleID(herdr.AgentExplain, s.ID)
	}
	return st, nil
}

func isNotFound(err error) bool {
	var notFound *herdr.AgentNotFoundError
	return errors.As(err, &notFound)
}

func mapNotFound(err error) error {
	if isNotFound(err) {
		return fmt.Errorf("%w: %v", agentrunner.ErrNotFound, err)
	}
	return err
}
