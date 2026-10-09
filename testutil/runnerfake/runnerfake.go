// Package runnerfake is an in-memory agentrunner.Runner for loop-level
// tests. Tests drive agent behavior through the Set* methods and inspect what
// was sent through Prompts and Answers.
package runnerfake

import (
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/elentok/gx/agentrunner"
)

type session struct {
	agentrunner.Session
	epic      string
	cwd       string
	status    agentrunner.Status
	promptErr error
	stopErr   error
	limited   bool
	resetAt   time.Time
	limitErr  error
	bgTask    bool
	prompts   []string
	answers   []agentrunner.Answer
}

// view is the status callers see: a running background task keeps an idle
// session working, as on a real runner.
func (ss *session) view() agentrunner.Status {
	st := ss.status
	if ss.bgTask && st.State == agentrunner.StateIdle {
		st.State = agentrunner.StateWorking
	}
	return st
}

type Runner struct {
	// PromptState is the state a session moves to after a delivered Prompt.
	// Defaults to working; set it to done for tests that don't care about the
	// turn in between.
	PromptState agentrunner.State
	// IDs names a new session of label. Nil numbers them fake-N and
	// session-N.
	IDs func(label string) (id, sessionID string)
	// Adopt makes Start return a live session of the same label and cwd, as
	// an adapter whose sessions outlive gx may, instead of ErrLabelTaken.
	Adopt bool

	mu        sync.Mutex
	nextID    int
	sessions  []*session // live sessions, in start order
	history   map[string]*session
	changed   chan struct{}
	healthErr error
	startErrs map[string]error
	failNext  map[string]failPrompts
}

type failPrompts struct {
	n   int
	err error
}

func NewRunner() *Runner {
	return &Runner{
		PromptState: agentrunner.StateWorking,
		history:     map[string]*session{},
		changed:     make(chan struct{}),
		startErrs:   map[string]error{},
		failNext:    map[string]failPrompts{},
	}
}

// notify wakes every Wait. Callers hold r.mu.
func (r *Runner) notify() {
	close(r.changed)
	r.changed = make(chan struct{})
}

// live returns the running session for s. Callers hold r.mu.
func (r *Runner) live(s agentrunner.Session) (*session, error) {
	for _, ss := range r.sessions {
		if ss.ID == s.ID {
			return ss, nil
		}
	}
	return nil, fmt.Errorf("%w: %s", agentrunner.ErrNotFound, s.Label)
}

func (r *Runner) byLabel(label string) *session {
	for _, ss := range r.sessions {
		if ss.Label == label {
			return ss
		}
	}
	panic(fmt.Sprintf("runnerfake: no live session %q", label))
}

func (r *Runner) Start(opts agentrunner.StartOptions) (agentrunner.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.startErrs[opts.Label]; err != nil {
		return agentrunner.Session{}, err
	}
	for _, ss := range r.sessions {
		if ss.Label == opts.Label {
			if r.Adopt && ss.cwd == opts.Cwd {
				return ss.Session, nil
			}
			return agentrunner.Session{}, fmt.Errorf("%w: %s", agentrunner.ErrLabelTaken, opts.Label)
		}
	}
	r.nextID++
	id, sessionID := fmt.Sprintf("fake-%d", r.nextID), fmt.Sprintf("session-%d", r.nextID)
	if r.IDs != nil {
		id, sessionID = r.IDs(opts.Label)
	}
	ss := &session{
		Session: agentrunner.Session{Label: opts.Label, ID: id, SessionID: sessionID},
		epic:    opts.Epic,
		cwd:     opts.Cwd,
	}
	ss.status = agentrunner.Status{State: agentrunner.StateIdle, SessionID: ss.SessionID}
	r.sessions = append(r.sessions, ss)
	r.history[opts.Label] = ss
	r.notify()
	return ss.Session, nil
}

func (r *Runner) Prompt(s agentrunner.Session, text string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ss, err := r.live(s)
	if err != nil {
		return err
	}
	if ss.status.State == agentrunner.StateBlocked {
		return fmt.Errorf("%w: %s", agentrunner.ErrNotReady, ss.status.BlockedReason)
	}
	if ss.promptErr != nil {
		return ss.promptErr
	}
	if f := r.failNext[s.Label]; f.n > 0 {
		f.n--
		r.failNext[s.Label] = f
		return f.err
	}
	ss.prompts = append(ss.prompts, text)
	ss.status.Turn++
	ss.status.State = r.PromptState
	r.notify()
	return nil
}

func (r *Runner) Status(s agentrunner.Session) (agentrunner.Status, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ss, err := r.live(s)
	if err != nil {
		return agentrunner.Status{}, err
	}
	return ss.view(), nil
}

func (r *Runner) Wait(s agentrunner.Session, states []agentrunner.State, timeout time.Duration) (agentrunner.Status, error) {
	deadline := time.After(timeout)
	for {
		r.mu.Lock()
		ss, err := r.live(s)
		if err != nil {
			r.mu.Unlock()
			return agentrunner.Status{}, err
		}
		st, changed := ss.view(), r.changed
		r.mu.Unlock()
		if slices.Contains(states, st.State) {
			return st, nil
		}
		select {
		case <-changed:
		case <-deadline:
			return st, agentrunner.ErrTimeout
		}
	}
}

func (r *Runner) Interrupt(s agentrunner.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ss, err := r.live(s)
	if err != nil {
		return err
	}
	ss.status.State = agentrunner.StateIdle
	ss.status.BlockedReason = ""
	r.notify()
	return nil
}

func (r *Runner) Stop(s agentrunner.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ss, err := r.live(s); err == nil && ss.stopErr != nil {
		return ss.stopErr
	}
	r.sessions = slices.DeleteFunc(r.sessions, func(ss *session) bool { return ss.ID == s.ID })
	r.notify()
	return nil
}

func (r *Runner) Find(label string) (agentrunner.Session, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, ss := range r.sessions {
		if ss.Label == label {
			return ss.Session, true, nil
		}
	}
	return agentrunner.Session{}, false, nil
}

func (r *Runner) List(epic string) ([]agentrunner.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []agentrunner.Session
	for _, ss := range r.sessions {
		if ss.epic == epic {
			out = append(out, ss.Session)
		}
	}
	return out, nil
}

func (r *Runner) RateLimit(s agentrunner.Session) (time.Time, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ss, err := r.live(s)
	if err != nil {
		return time.Time{}, false, err
	}
	if ss.limitErr != nil {
		return time.Time{}, false, ss.limitErr
	}
	return ss.resetAt, ss.limited, nil
}

func (r *Runner) Answer(s agentrunner.Session, a agentrunner.Answer) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ss, err := r.live(s)
	if err != nil {
		return err
	}
	if ss.status.State != agentrunner.StateBlocked {
		return fmt.Errorf("%w: not blocked", agentrunner.ErrNotReady)
	}
	ss.answers = append(ss.answers, a)
	ss.status.State = agentrunner.StateWorking
	ss.status.BlockedReason = ""
	r.notify()
	return nil
}

func (r *Runner) Healthy() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.healthErr
}

// SetState moves the live session label to state; reason is the blocked
// reason and is ignored for other states.
func (r *Runner) SetState(label string, state agentrunner.State, reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ss := r.byLabel(label)
	ss.status.State = state
	ss.status.BlockedReason = ""
	if state == agentrunner.StateBlocked {
		ss.status.BlockedReason = reason
	}
	r.notify()
}

// SetPromptErr makes every Prompt to label fail with err until cleared with
// nil.
func (r *Runner) SetPromptErr(label string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byLabel(label).promptErr = err
}

// FailNextPrompts makes the next n Prompts to label fail with err, across
// sessions, so a test can fail a prompt before its session exists.
func (r *Runner) FailNextPrompts(label string, n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failNext[label] = failPrompts{n: n, err: err}
}

// SetStartErr makes every Start of label fail with err (e.g. ErrLabelTaken)
// until cleared with nil.
func (r *Runner) SetStartErr(label string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.startErrs[label] = err
}

// SetStopErr makes every Stop of the live session label fail with err, and
// leave it running, until cleared with nil.
func (r *Runner) SetStopErr(label string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byLabel(label).stopErr = err
}

// SetRateLimit marks label as rate limited until resetAt; a zero time clears
// it.
func (r *Runner) SetRateLimit(label string, resetAt time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ss := r.byLabel(label)
	ss.resetAt, ss.limited = resetAt, !resetAt.IsZero()
}

// SetLimitedUnknownReset marks label as rate limited with no known reset
// time, as when the agent's message gave none.
func (r *Runner) SetLimitedUnknownReset(label string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ss := r.byLabel(label)
	ss.resetAt, ss.limited = time.Time{}, true
}

// SetRateLimitErr makes RateLimit on label fail with err (e.g. a wrapped
// ErrContextExhausted) until cleared with nil.
func (r *Runner) SetRateLimitErr(label string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byLabel(label).limitErr = err
}

// SetBackgroundTask starts or ends a background task of label.
func (r *Runner) SetBackgroundTask(label string, running bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byLabel(label).bgTask = running
	r.notify()
}

func (r *Runner) SetHealthErr(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.healthErr = err
}

// Prompts returns the delivered prompts of the latest session started with
// label, including a stopped one.
func (r *Runner) Prompts(label string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ss := r.history[label]; ss != nil {
		return slices.Clone(ss.prompts)
	}
	return nil
}

// Answers returns the answers sent to the latest session started with label,
// including a stopped one.
func (r *Runner) Answers(label string) []agentrunner.Answer {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ss := r.history[label]; ss != nil {
		return slices.Clone(ss.answers)
	}
	return nil
}
