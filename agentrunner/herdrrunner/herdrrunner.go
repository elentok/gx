// Package herdrrunner implements agentrunner.Runner on herdr: one workspace
// per epic, one tab and agent per session, with the agent named after the
// session label. Session.ID is the agent's pane id.
package herdrrunner

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/herdr"
)

// pollInterval spaces AgentWait retries when herdr answers a wait with a
// timeout before the caller's own deadline.
const pollInterval = 20 * time.Millisecond

type Runner struct {
	mu sync.Mutex
	// turns counts prompts the agent picked up, keyed by label. herdr's
	// StateChangeSeq also advances when a turn ends, so it can't be Turn as is.
	turns map[string]*turn
}

type turn struct {
	count   int
	lastSeq int
}

var _ agentrunner.Runner = (*Runner)(nil)
var _ agentrunner.HealthChecker = (*Runner)(nil)

func New() *Runner {
	return &Runner{turns: map[string]*turn{}}
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
	var taken *herdr.AgentNameTakenError
	var notReady *herdr.AgentNotReadyError
	switch {
	case errors.As(err, &taken):
		_ = herdr.TabClose(tab.TabID)
		// Our own earlier launch is still running: adopt it.
		if taken.CandidateCwd != opts.Cwd {
			return agentrunner.Session{}, fmt.Errorf("%w: %s", agentrunner.ErrLabelTaken, taken.Message)
		}
		agent, err = herdr.AgentGet(opts.Label)
		if err != nil {
			return agentrunner.Session{}, err
		}
	case errors.As(err, &notReady):
		// Only the trust_directory dialog is answered: gx raised it by
		// launching in a new directory. Any other dialog is not ours to answer.
		if rule := matchedRuleID(tab.RootPaneID); rule != "trust_directory" {
			return agentrunner.Session{}, fmt.Errorf("%w: %s blocked on dialog %q", agentrunner.ErrNotReady, opts.Label, rule)
		}
		if err := herdr.AgentSendKeys(tab.RootPaneID, "enter"); err != nil {
			return agentrunner.Session{}, err
		}
		agent.PaneID = tab.RootPaneID
	case err != nil:
		return agentrunner.Session{}, err
	}
	s := agentrunner.Session{Label: opts.Label, ID: agent.PaneID, SessionID: agent.AgentSession}
	// Drop a stopped predecessor's count; the wait's status read starts anew.
	r.mu.Lock()
	delete(r.turns, opts.Label)
	r.mu.Unlock()
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
	agent, err := herdr.AgentPrompt(herdr.AgentPromptOptions{Target: s.ID, Text: text})
	var blocked *herdr.AgentBlockedError
	if errors.As(err, &blocked) {
		return fmt.Errorf("%w: %s", agentrunner.ErrNotReady, blocked.Message)
	}
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	t := r.turnLocked(s.Label, st.seq)
	if agent.StateChangeSeq <= t.lastSeq {
		return agentrunner.ErrNotDelivered
	}
	t.count++
	t.lastSeq = agent.StateChangeSeq
	return nil
}

func (r *Runner) Status(s agentrunner.Session) (agentrunner.Status, error) {
	st, err := r.status(s)
	return st.Status, err
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
		if err != nil && !isPollTimeout(err) {
			return agentrunner.Status{}, mapNotFound(err)
		}
		if err == nil {
			st, err := r.status(s)
			return st.Status, err
		}
		if time.Now().After(deadline) {
			st, err := r.status(s)
			if err != nil {
				return agentrunner.Status{}, err
			}
			return st.Status, agentrunner.ErrTimeout
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
	delete(r.turns, s.Label)
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

// List returns the epic workspace's sessions, taking tab labels as agent
// names the way Start creates them.
func (r *Runner) List(epic string) ([]agentrunner.Session, error) {
	wsID, err := herdr.FindWorkspace(epic)
	if err != nil || wsID == "" {
		return nil, err
	}
	tabs, err := herdr.TabList(wsID)
	if err != nil {
		return nil, err
	}
	var out []agentrunner.Session
	for _, tab := range tabs {
		s, ok, err := r.Find(tab.Label)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, s)
		}
	}
	return out, nil
}

// RateLimit is not wired to herdr yet; see the sibling rate-limit ticket.
func (r *Runner) RateLimit(agentrunner.Session) (time.Time, bool, error) {
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

type status struct {
	agentrunner.Status
	seq int
}

func (r *Runner) status(s agentrunner.Session) (status, error) {
	agent, err := herdr.AgentGet(s.ID)
	if err != nil {
		return status{}, mapNotFound(err)
	}
	st := status{Status: agentrunner.Status{State: agentrunner.State(agent.AgentStatus), SessionID: agent.AgentSession}, seq: agent.StateChangeSeq}
	if !slices.Contains([]agentrunner.State{agentrunner.StateWorking, agentrunner.StateIdle, agentrunner.StateDone, agentrunner.StateBlocked}, st.State) {
		st.State = agentrunner.StateWorking
	}
	if st.State == agentrunner.StateBlocked {
		st.BlockedReason = matchedRuleID(s.ID)
		if st.BlockedReason == "" {
			st.BlockedReason = "blocked"
		}
	}
	r.mu.Lock()
	st.Turn = r.turnLocked(s.Label, agent.StateChangeSeq).count
	r.mu.Unlock()
	return st, nil
}

// turnLocked returns label's turn counter, starting one at seq for a session
// this runner did not start (e.g. found after a gx restart).
func (r *Runner) turnLocked(label string, seq int) *turn {
	t, ok := r.turns[label]
	if !ok {
		t = &turn{lastSeq: seq}
		r.turns[label] = t
	}
	return t
}

func matchedRuleID(pane string) string {
	res, err := herdr.AgentExplain(pane)
	if err != nil {
		return ""
	}
	return res.MatchedRuleID
}

// isPollTimeout mirrors ralphloop's: herdr reports a wait or prompt that ran
// out of time under several messages.
func isPollTimeout(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timed out") ||
		strings.Contains(msg, "agent_prompt_stalled") ||
		strings.Contains(msg, "no observed state change")
}

func isNotFound(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "not found")
}

func mapNotFound(err error) error {
	if isNotFound(err) {
		return fmt.Errorf("%w: %v", agentrunner.ErrNotFound, err)
	}
	return err
}
