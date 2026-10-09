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
	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/herdr"
)

// pollInterval spaces AgentWait retries when herdr answers a wait with a
// timeout before the caller's own deadline.
const pollInterval = 20 * time.Millisecond

type Runner struct {
	// BackgroundTasks reports whether the agent behind s still has a
	// background task running. herdr shows such an agent idle, but the task
	// will wake it, so the runner reports it working. Nil means no gating.
	BackgroundTasks func(s agentrunner.Session) bool
	// CodexQuota is checked before the pane for Codex sessions. Nil skips it.
	CodexQuota CodexQuotaReader

	mu sync.Mutex
	// started remembers what RateLimit needs from Start, keyed by label.
	started map[string]agentrunner.StartOptions
}

var _ agentrunner.Runner = (*Runner)(nil)
var _ agentrunner.HealthChecker = (*Runner)(nil)

// New gates on Claude transcript background tasks, unless the home directory
// can't be found.
func New() *Runner {
	r := &Runner{started: map[string]agentrunner.StartOptions{}, CodexQuota: codexsession.LastRateLimit}
	if home, err := os.UserHomeDir(); err == nil {
		r.BackgroundTasks = ClaudeBackgroundTasks(home, time.Now)
	}
	return r
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
		recovery, err := RecoverStart(err, opts.Label, opts.Cwd, tab.RootPaneID, herdr.AgentExplain, herdr.AgentSendKeys)
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
	st, err := r.status(s)
	if err != nil {
		return err
	}
	if st.State == agentrunner.StateBlocked {
		return agentrunner.ErrNotReady
	}
	compact := text == "/compact"
	until := []string{"working"}
	if compact {
		// Codex asks to confirm a compact before it starts.
		until = append(until, "blocked")
	}
	prompt := PromptWithNudge(herdr.AgentPrompt, herdr.AgentSendKeys, herdr.AgentWait, herdr.AgentRead, time.Now)
	agent, err := prompt(herdr.AgentPromptOptions{Target: s.ID, Text: text, Wait: true, Until: until})
	var blocked *herdr.AgentBlockedError
	switch {
	case errors.As(err, &blocked):
		return fmt.Errorf("%w: %s", agentrunner.ErrNotReady, blocked.Message)
	case errors.Is(err, ErrStuckSubmission):
		return fmt.Errorf("%w: %w", agentrunner.ErrNotDelivered, err)
	case err != nil:
		return err
	}
	if agent.StateChangeSeq <= st.Turn {
		return agentrunner.ErrNotDelivered
	}
	if compact {
		return r.confirmCompact(s, agent.AgentStatus)
	}
	return nil
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
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		_, err := herdr.AgentWait(herdr.AgentWaitOptions{Target: s.ID, Until: until, TimeoutMs: max(1, int(remaining.Milliseconds()))})
		if err != nil && !IsPollTimeout(err) {
			return agentrunner.Status{}, mapNotFound(err)
		}
		if err == nil {
			// herdr's state can still be overridden by a background task.
			st, err := r.status(s)
			if err != nil || slices.Contains(states, st.State) {
				return st, err
			}
		}
		if time.Now().After(deadline) {
			st, err := r.status(s)
			if err != nil {
				return agentrunner.Status{}, err
			}
			return st, agentrunner.ErrTimeout
		}
		time.Sleep(min(pollInterval, remaining))
	}
}

func (r *Runner) Interrupt(s agentrunner.Session) error {
	return mapNotFound(herdr.AgentSendKeys(s.ID, "ctrl+c"))
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
	token, limited := DetectRateLimit(text)
	if !limited {
		return time.Time{}, false, nil
	}
	if d, ok := SecondsUntilReset(token, now); ok {
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

func (r *Runner) Answer(s agentrunner.Session, a agentrunner.Answer) error {
	st, err := r.status(s)
	if err != nil {
		return err
	}
	if st.State != agentrunner.StateBlocked {
		return agentrunner.ErrNotReady
	}
	keys := []string{"enter"}
	switch {
	case a.Text != "":
		keys = []string{a.Text, "enter"}
	case a.Decision == agentrunner.DecisionDeny:
		keys = []string{"escape"}
	}
	return herdr.AgentSendKeys(s.ID, keys...)
}

// status reports herdr's StateChangeSeq as Turn: herdr keeps it, so it holds
// across a gx restart, and it advances on every turn start (and end).
func (r *Runner) status(s agentrunner.Session) (agentrunner.Status, error) {
	agent, err := herdr.AgentGet(s.ID)
	if err != nil {
		return agentrunner.Status{}, mapNotFound(err)
	}
	st := agentrunner.Status{State: agentrunner.State(agent.AgentStatus), Turn: agent.StateChangeSeq, SessionID: agent.AgentSession}
	switch st.State {
	case agentrunner.StateWorking, agentrunner.StateBlocked:
	case agentrunner.StateIdle, agentrunner.StateDone:
		// s may predate herdr learning the agent's session id.
		probed := s
		if agent.AgentSession != "" {
			probed.SessionID = agent.AgentSession
		}
		if r.BackgroundTasks != nil && r.BackgroundTasks(probed) {
			st.State = agentrunner.StateWorking
		}
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
