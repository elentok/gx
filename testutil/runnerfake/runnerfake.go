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
	epic           string
	cwd            string
	args           []string
	kind           agentrunner.Kind
	resumeID       string
	status         agentrunner.Status
	promptErr      error
	answerErr      error
	interruptErr   error
	interruptDelay time.Duration
	stopErr        error
	limited        bool
	resetAt        time.Time
	limitErr       error
	diedErr        error
	bgTask         bool
	prompts        []string
	interrupts     int
	answers        []agentrunner.Answer
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
	// ResumeErr fails every Start that asks to resume a session, as a pty
	// runner does when claude cannot resume it.
	ResumeErr error

	mu        sync.Mutex
	starts    []agentrunner.StartOptions // every Start attempt, failed ones too
	nextID    int
	sessions  []*session // live sessions, in start order
	history   map[string]*session
	changed   chan struct{}
	healthErr error
	startErrs map[string]error
	failNext  map[string]failPrompts
	dieAfter  map[string]error
	input     map[string]chan struct{}
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
		dieAfter:    map[string]error{},
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
	r.starts = append(r.starts, opts)
	if err := r.startErrs[opts.Label]; err != nil {
		return agentrunner.Session{}, err
	}
	if opts.ResumeSessionID != "" && r.ResumeErr != nil {
		return agentrunner.Session{}, r.ResumeErr
	}
	for _, ss := range r.sessions {
		if ss.Label == opts.Label {
			if r.Adopt && !opts.NoAdopt && ss.cwd == opts.Cwd {
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
		Session:  agentrunner.Session{Label: opts.Label, ID: id, SessionID: sessionID},
		epic:     opts.Epic,
		cwd:      opts.Cwd,
		args:     slices.Clone(opts.Args),
		kind:     opts.Kind,
		resumeID: opts.ResumeSessionID,
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
	if err := r.dieAfter[s.Label]; err != nil {
		delete(r.dieAfter, s.Label)
		ss.diedErr = err
	}
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
	if ss.diedErr != nil {
		return agentrunner.Status{}, ss.diedErr
	}
	return ss.view(), nil
}

func (r *Runner) Wait(s agentrunner.Session, states []agentrunner.State, timeout time.Duration) (agentrunner.Status, error) {
	deadline := time.After(timeout)
	for {
		r.mu.Lock()
		ss, err := r.live(s)
		if err == nil {
			err = ss.diedErr
		}
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

// PersonInput implements agentrunner.PersonInputter: events fire on each
// SendPersonInput for the session's label.
func (r *Runner) PersonInput(s agentrunner.Session) (<-chan struct{}, func(), error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.live(s); err != nil {
		return nil, nil, err
	}
	if r.input == nil {
		r.input = map[string]chan struct{}{}
	}
	ch, ok := r.input[s.Label]
	if !ok {
		ch = make(chan struct{}, 1)
		r.input[s.Label] = ch
	}
	return ch, func() {}, nil
}

// SendPersonInput reports a person's key on label's session, as a pty runner
// does. Events collapse while the receiver is busy.
func (r *Runner) SendPersonInput(label string) {
	r.mu.Lock()
	ch := r.input[label]
	r.mu.Unlock()
	select {
	case ch <- struct{}{}:
	default:
	}
}

// StopAll drops every session, so a Wait on one ends with ErrNotFound. A test
// that stops its server calls it: agents outlive the server, and a settle still
// waiting on one would otherwise run until the test's own deadline.
func (r *Runner) StopAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions = nil
	r.notify()
}

func (r *Runner) Interrupt(s agentrunner.Session) error {
	r.mu.Lock()
	ss, err := r.live(s)
	if err != nil {
		r.mu.Unlock()
		return err
	}
	delay, failure := ss.interruptDelay, ss.interruptErr
	r.mu.Unlock()
	// The delay runs unlocked so other calls on the runner proceed meanwhile.
	time.Sleep(delay)
	if failure != nil {
		return failure
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	ss.interrupts++
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

func (r *Runner) Name() string { return "fake" }

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
	if ss.answerErr != nil {
		return ss.answerErr
	}
	if p := ss.status.Pending; a.RequestID != "" && (p == nil || p.ID != a.RequestID) {
		return fmt.Errorf("%w: %s", agentrunner.ErrRequestMismatch, a.RequestID)
	}
	ss.answers = append(ss.answers, a)
	ss.status.State = agentrunner.StateWorking
	ss.status.BlockedReason = ""
	ss.status.Pending = nil
	r.notify()
	return nil
}

// SetPending blocks the live session label on the open request p, the way a
// headless can_use_tool request does. A nil p clears it.
func (r *Runner) SetPending(label string, p *agentrunner.Pending) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ss := r.byLabel(label)
	ss.status.Pending = p
	if p != nil {
		ss.status.State = agentrunner.StateBlocked
		ss.status.BlockedReason = fmt.Sprintf("permission to use %s: %s", p.Tool, p.Input)
	}
	r.notify()
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
	ss.status.BlockedReason, ss.status.Error = "", ""
	ss.status.Pending = nil
	if state == agentrunner.StateBlocked {
		ss.status.BlockedReason = reason
	}
	r.notify()
}

// SetErrored makes label blocked on an agent error: Status reports
// StateBlocked with Error text and no BlockedReason, as a Codex turn that
// ended in an error does. SetState clears it.
func (r *Runner) SetErrored(label, text string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ss := r.byLabel(label)
	ss.status.State = agentrunner.StateBlocked
	ss.status.BlockedReason, ss.status.Pending, ss.status.Error = "", nil, text
	r.notify()
}

// SetAnswerErr makes every Answer to label fail with err until cleared with
// nil.
func (r *Runner) SetAnswerErr(label string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byLabel(label).answerErr = err
}

// SetPromptErr makes every Prompt to label fail with err until cleared with
// nil.
func (r *Runner) SetPromptErr(label string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byLabel(label).promptErr = err
}

// SetInterruptErr makes every Interrupt of label fail with err until cleared
// with nil.
func (r *Runner) SetInterruptErr(label string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byLabel(label).interruptErr = err
}

// SetInterruptDelay makes every Interrupt of label take d before it acts (or
// fails), until cleared with 0.
func (r *Runner) SetInterruptDelay(label string, d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byLabel(label).interruptDelay = d
}

// FailNextPrompts makes the next n Prompts to label fail with err, across
// sessions, so a test can fail a prompt before its session exists.
func (r *Runner) FailNextPrompts(label string, n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failNext[label] = failPrompts{n: n, err: err}
}

// DieAfterPrompt makes the next delivered Prompt to label kill its session:
// from then on Status and Wait fail with err (e.g. agentrunner.SessionDied).
// Like FailNextPrompts, it can be set before the session exists.
func (r *Runner) DieAfterPrompt(label string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dieAfter[label] = err
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

// Interrupts counts the successful Interrupts of the latest session started
// with label, including a stopped one.
func (r *Runner) Interrupts(label string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ss := r.history[label]; ss != nil {
		return ss.interrupts
	}
	return 0
}

// Args returns the launch args of the latest session started with label,
// including a stopped one.
func (r *Runner) Args(label string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ss := r.history[label]; ss != nil {
		return slices.Clone(ss.args)
	}
	return nil
}

// Kind returns the agent kind the latest session started with label was asked
// for, including a stopped one.
func (r *Runner) Kind(label string) agentrunner.Kind {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ss := r.history[label]; ss != nil {
		return ss.kind
	}
	return ""
}

// ResumeSessionID returns the ResumeSessionID the latest session started with
// label was given, including a stopped one.
func (r *Runner) ResumeSessionID(label string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ss := r.history[label]; ss != nil {
		return ss.resumeID
	}
	return ""
}

// ResumeIDs is the ResumeSessionID of each Start attempt of label, in order,
// failed attempts included.
func (r *Runner) ResumeIDs(label string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var ids []string
	for _, o := range r.starts {
		if o.Label == label {
			ids = append(ids, o.ResumeSessionID)
		}
	}
	return ids
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
