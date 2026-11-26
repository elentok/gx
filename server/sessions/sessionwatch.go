package sessions

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/subagent"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/sound"
)

// sessionWatches runs one watchSession per live session for the server's
// lifetime.
type sessionWatches struct {
	mu      sync.Mutex
	stopped bool
	Stop    chan struct{}
	running sync.WaitGroup
	// MetaMu serializes the watches' meta read-modify-writes.
	MetaMu sync.Mutex
}

func NewSessionWatches() *sessionWatches {
	return &sessionWatches{Stop: make(chan struct{})}
}

func (w *sessionWatches) Start(watch func()) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped {
		return
	}
	w.running.Add(1)
	go func() {
		defer w.running.Done()
		watch()
	}()
}

// Context is cancelled when the watches stop. release frees it early.
func (w *sessionWatches) Context() (ctx context.Context, release context.CancelFunc) {
	ctx, release = context.WithCancel(context.Background())
	go func() {
		select {
		case <-w.Stop:
			release()
		case <-ctx.Done():
		}
	}()
	return ctx, release
}

func (w *sessionWatches) IsStopped() bool {
	select {
	case <-w.Stop:
		return true
	default:
		return false
	}
}

// Close ends every watch, which records nothing for its stream's end, and
// waits for them.
func (w *sessionWatches) Close() {
	w.mu.Lock()
	if !w.stopped {
		w.stopped = true
		close(w.Stop)
	}
	w.mu.Unlock()
	w.running.Wait()
}

// watchSession records the session's status and conversation id from its
// holder's watch stream until the stream ends. A stream that ends without an
// exit while the server is up means the holder died: Exited crashed, with no
// retry. Any exit before crashUntil is crashed too. A reconnect attaches to a
// session that may already be mid-turn, so only an idle-to-busy edge it sees
// makes a turn Live; a watch from launch sees every turn start.
func (mg *Manager) watchSession(dir, id string, run int, crashUntil time.Time, reconnect bool) {
	update := func(change func(*Meta) bool) {
		mg.UpdateSessionMetaOrWarn(dir, id, func(m *Meta) bool { return m.Run == run && change(m) })
	}
	recordExit := func(exit string) {
		// A switched agent that ends before idle has failed its switch.
		if sw, ok := mg.switches.take(id, run); ok {
			NewResumer(mg).failSwitch(dir, id, run, sw)
			return
		}
		if time.Now().Before(crashUntil) {
			exit = wire.SessionExitCrashed
		}
		update(func(m *Meta) bool { return mg.recordSessionExit(m, exit) })
	}
	// A switch that never reached idle ends with its run.
	defer mg.switches.take(id, run)
	stream, err := mg.openSessionWatch(dir)
	if err != nil {
		if !mg.Watches.IsStopped() {
			mg.log.Warn("session watch", "session", id, "err", err)
			recordExit(savedSessionExit(dir))
		}
		return
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-mg.Watches.Stop:
		case <-done:
		}
		_ = stream.Close()
	}()
	first := mg.prompter.NewFirstPrompt(dir, id, run)
	defer first.Stop()
	poll := mg.chat.NewPoll(dir, id)
	defer poll.Close()
	// settle is closed to end the wait of the last idle edge's settleTurn.
	var settle chan struct{}
	endSettle := func() {
		if settle != nil {
			close(settle)
			settle = nil
		}
	}
	defer endSettle()
	seenBusy, sawIdle, attaching := false, !reconnect, reconnect
	for {
		msg, err := stream.Next()
		if err != nil {
			if !mg.Watches.IsStopped() {
				recordExit(savedSessionExit(dir))
			}
			return
		}
		switch msg.Type {
		case holder.StreamStatus:
			st := SessionStatusOf(msg.Status)
			var prompt, signal string
			var design chat.DesignRef
			idleEdge, unconfirmed := false, false
			// An idle whose turn start this watch did not see (an idle-to-busy
			// edge) is not Live, e.g. a turn already running at connect.
			live := seenBusy
			// A turn also ends while Working with Background: the agent is
			// waiting for input even though its background task is still alive.
			busy := !st.Promptable()
			if busy {
				endSettle()
			}
			seenBusy = busy && (seenBusy || sawIdle)
			sawIdle = !busy
			// Before the update publishes the new status, so the poll's baseline
			// is no later than the page's read of the chat.
			poll.SetActive(st.State == wire.SessionStarting || st.State == wire.SessionWorking)
			update(func(m *Meta) bool {
				// A stop records the exit itself, and a status the stream sent
				// before it must not revive the session.
				if m.Status.State == wire.SessionExited {
					return false
				}
				changed := false
				if prev := m.Info().Status; prev != st {
					if raisesUnread(prev.State, st.State, msg.Status.Cancelled) {
						mg.raiseUnread(m)
						mg.TriggerBanner(m.ID, unreadSound(st.State), "")
					}
					// The released dialog is gone once the turn ends. Not on any
					// other exit from Blocked: the hook ending passes through a
					// busy status before Claude's own dialog shows as Blocked
					// again, and that dialog must stay released.
					if prev.State == wire.SessionBlocked && st.State == wire.SessionIdle {
						mg.ForgetReleasedAsks(m.ID)
					}
					m.Status, m.StatusChangedAt = st, time.Now()
					changed = true
					// A switched agent that is idle is alive: the record takes the switch.
					if st.State == wire.SessionIdle {
						if sw, ok := mg.switches.take(id, run); ok {
							if sw.back {
								mg.switches.setFailure(id, switchFailure{attempted: sw.attempted, old: sw.model})
							} else {
								recordSwitchNow(mg, m, sw.model, sw.effort)
							}
						}
					}
					if st.Promptable() {
						m.ReportedStep, m.ReportedTitle = "", ""
					}
					if st.Promptable() && !prev.Promptable() {
						idleEdge = true
						if m.IsStepped() {
							design = chat.DesignRef{Conversation: m.ConversationID, Ticket: m.Ticket, Flow: m.FlowRef(), Live: live}
						}
					}
				}
				// The first status of an attach also checks, so a restart between
				// idle and the first message keeps the rollback.
				// A harness may also check on a non-idle state it allows (Codex's
				// Blocked), on its edge.
				unconfirmed = m.SwitchUnconfirmed && firstReplyReaderFor(m.Harness).StateOK(st.State) &&
					(idleEdge || (st.State != wire.SessionIdle && changed) || attaching)
				if msg.ConversationID != "" && msg.ConversationID != m.ConversationID {
					mg.Conv.SaveTotals(m.ConvSession(), m.ConversationID)
					harness.CarryEditorOverride(m.ConversationID, msg.ConversationID)
					m.ConversationID = msg.ConversationID
					mg.Conv.Record(m.ConvSession())
					changed = true
				}
				if m.AutoName && msg.DisplayName != "" && msg.DisplayName != m.Name {
					m.Name = msg.DisplayName
					changed = true
				}
				if m.FirstPrompt != "" {
					if sig := first.Observe(st.State, msg.Ready); sig != "" {
						prompt, signal, m.FirstPrompt = m.FirstPrompt, sig, ""
						changed = true
					}
				}
				return changed
			})
			first.NoteState(st.State)
			mg.NoteFailure(id)
			if st.State == wire.SessionBlocked {
				mg.NoteQuestionCards(id)
			}
			// Marked before the Design check reads the transcript.
			mark, marked := chat.TurnMark{}, false
			if idleEdge {
				mark, marked = mg.prompter.MarkTurn(dir, id)
			}
			if design.Conversation != "" {
				mg.prompter.DetectDesignActionable(dir, id, design)
			}
			if marked {
				endSettle()
				cancel := make(chan struct{})
				settle = cancel
				mg.Watches.Start(func() { mg.prompter.SettleTurn(dir, id, mark, design, cancel) })
			}
			if unconfirmed {
				if settle == nil {
					settle = make(chan struct{})
				}
				cancel := settle
				mg.Watches.Start(func() { NewResumer(mg).confirmSwitch(dir, id, run, cancel) })
			}
			attaching = false
			if prompt != "" {
				first.Start(signal, prompt)
			}
		case holder.StreamInput:
			first.PersonInput()
			// Typing during a countdown stops only the countdown: the run-end
			// Continue stays raised.
			stopped := mg.prompter.CancelAutoAdvance(id, "Auto-advance stopped: you typed")
			update(func(m *Meta) bool {
				changed := m.AutoSends != 0
				m.AutoSends = 0
				if stopped || m.Actionable.GxRaised {
					return changed
				}
				changed = changed || m.Actionable.Reason != ""
				m.Actionable = wire.SessionActionable{}
				return changed
			})
		case holder.StreamExit:
			recordExit(sessionExitOf(msg.ExitCode))
			return
		}
	}
}

func sessionExitOf(code int) string {
	if code == 0 {
		return wire.SessionExitClean
	}
	return wire.SessionExitCrashed
}

// savedSessionExit is the exit the holder saved before its socket went, for
// a child that exited before the watch connected; without one the holder
// died.
func savedSessionExit(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, holder.ExitCodeFile))
	if err != nil {
		return wire.SessionExitCrashed
	}
	code, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return wire.SessionExitCrashed
	}
	return sessionExitOf(code)
}

// openSessionWatch waits for a just-started holder's socket, then opens its
// watch stream.
func (mg *Manager) openSessionWatch(dir string) (*holder.HolderWatch, error) {
	c, err := mg.dialNewHolder(dir)
	if errors.Is(err, errHolderExited) {
		return nil, errors.New("holder exited before its watch opened")
	}
	if err != nil {
		return nil, err
	}
	if !c.Supports(holder.OpWatch) {
		return nil, errors.New("holder does not serve watch")
	}
	return c.Watch()
}

var errHolderExited = errors.New("holder exited")

// dialNewHolder waits for a just-started holder's socket, until the holder
// saves an exit, holderDialTimeout passes or the server stops.
func (mg *Manager) dialNewHolder(dir string) (*holder.HolderClient, error) {
	deadline := time.Now().Add(holderDialTimeout)
	for {
		c, err := holder.DialHolder(dir)
		if err == nil {
			return c, nil
		}
		if _, statErr := os.Stat(filepath.Join(dir, holder.ExitCodeFile)); statErr == nil {
			return nil, errHolderExited
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		select {
		case <-mg.Watches.Stop:
			return nil, err
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// recordSessionExit records the first exit seen and drops an untyped first
// prompt; a stopped session exits clean whatever the signal left as its code.
func (mg *Manager) recordSessionExit(m *Meta, exit string) bool {
	if m.Status.State == wire.SessionExited {
		return false
	}
	if m.Stopped {
		exit = wire.SessionExitClean
	} else {
		mg.raiseUnread(m)
		// A Done Design exiting cleanly is expected: no banner.
		if exit != wire.SessionExitClean || !designDone(mg.projects, *m) {
			mg.TriggerBanner(m.ID, sound.Done, "")
		}
	}
	mg.markExited(m, exit)
	m.FirstPrompt = ""
	// The unknown-command card outlives the person's Stop so Resend works after a restart.
	if !m.Stopped || m.Actionable.Kind != wire.DeliveryUnknownCommand {
		m.Actionable = wire.SessionActionable{}
	}
	return true
}

// markExited moves m to Exited with exit, leaving Unread, FirstPrompt and
// Actionable as saved and firing nothing.
func (mg *Manager) markExited(m *Meta, exit string) {
	mg.Conv.SaveTotals(m.ConvSession(), m.ConversationID)
	mg.ForgetReleasedAsks(m.ID)
	m.Status, m.StatusChangedAt, m.Exit = wire.SessionStatus{State: wire.SessionExited}, time.Now(), exit
	m.RestoreCrashed = exit == wire.SessionExitCrashed && !m.RestoredAt.IsZero() && time.Since(m.RestoredAt) < chat.RestoreCrashWindow
}

// raisesUnread reports whether a move from one live state to another needs
// the person: a finished turn, a block or a rate limit. Idle from Starting is
// the harness coming up, not a turn.
func raisesUnread(from, to string, cancelled bool) bool {
	if from == to {
		return false
	}
	switch to {
	case wire.SessionIdle:
		return !cancelled && from != wire.SessionStarting
	case wire.SessionBlocked, wire.SessionRateLimited:
		return true
	}
	return false
}

// raiseUnread sets m's Unread unless a focused TUI has it on screen.
func (mg *Manager) raiseUnread(m *Meta) {
	if !mg.focus.onScreen(m.ID) {
		m.Unread.Unread = true
	}
}

// sessionWrite adapts a session verb to a POST handler. Refusals are results,
// so they answer 200.
func SessionWrite(do func(wire.SessionRequest) (wire.SessionResult, error)) http.HandlerFunc {
	return sessionWriteAs(nil, do)
}

func sessionWriteAs(check func(wire.SessionRequest) (wire.SessionResult, error), do func(wire.SessionRequest) (wire.SessionResult, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req wire.SessionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if check != nil {
			res, err := check(req)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if res.Refused {
				wire.WriteJSON(w, res)
				return
			}
		}
		res, err := do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		wire.WriteJSON(w, res)
	}
}

// subagentWatches are the open subagent page watches, by the client that
// opened them.
type subagentWatches struct {
	mu sync.Mutex
	by map[string]map[[2]string]*chat.SubagentWatch // client -> (session, call)
}

// put keeps w for the client, ending the watch it replaces.
func (s *subagentWatches) put(client string, key [2]string, w *chat.SubagentWatch) {
	s.mu.Lock()
	if s.by == nil {
		s.by = map[string]map[[2]string]*chat.SubagentWatch{}
	}
	if s.by[client] == nil {
		s.by[client] = map[[2]string]*chat.SubagentWatch{}
	}
	old := s.by[client][key]
	s.by[client][key] = w
	s.mu.Unlock()
	if old != nil {
		old.Close()
	}
}

func (s *subagentWatches) end(client string, key [2]string) {
	s.mu.Lock()
	w := s.by[client][key]
	delete(s.by[client], key)
	s.mu.Unlock()
	if w != nil {
		w.Close()
	}
}

// endClient ends every watch of a client whose last event stream closed.
func (s *subagentWatches) endClient(client string) {
	s.mu.Lock()
	ws := s.by[client]
	delete(s.by, client)
	s.mu.Unlock()
	for _, w := range ws {
		w.Close()
	}
}

// cardWatches are the session watches of the clients whose page shows a working
// card.
type cardWatches struct {
	mu sync.Mutex            // guards by and users, never held over a watch's start or end
	by map[string]*cardWatch // client
}

type cardWatch struct {
	users int // holders and waiters of mu

	mu      sync.Mutex // serializes one client's syncs; guards the fields below
	session string
	w       *chat.SubagentWatch // nil when the client has no watch
}

// lock returns the client's entry, locked. release unlocks it and drops an
// entry with no watch that nobody else waits for.
func (s *cardWatches) lock(client string) (c *cardWatch, release func()) {
	s.mu.Lock()
	if s.by == nil {
		s.by = map[string]*cardWatch{}
	}
	c = s.by[client]
	if c == nil {
		c = &cardWatch{}
		s.by[client] = c
	}
	c.users++
	s.mu.Unlock()
	c.mu.Lock()
	return c, func() {
		defer c.mu.Unlock()
		s.mu.Lock()
		defer s.mu.Unlock()
		c.users--
		if c.users == 0 && c.w == nil {
			delete(s.by, client)
		}
	}
}

// syncCardWatch starts or stops the client's session watch to match its latest
// focus report. It reads the report under the client's lock, so two quick
// reports cannot leave a stale watch.
func syncCardWatch(mg *Manager, client string) {
	c, release := mg.cardWatches.lock(client)
	defer release()
	session, watching := mg.focus.watchedSession(client)
	if c.w != nil {
		if watching && c.session == session {
			return
		}
		c.w.Close()
		c.w = nil
	}
	if !watching {
		return
	}
	dir, _, res, err := mg.SessionDir(session)
	if res.Refused || err != nil {
		return
	}
	c.session, c.w = session, chat.WatchSession(mg.chat, session, dir, func() {
		mg.events.PublishEvent(wire.Event{Type: wire.EventSessionSubagentChanged, Address: session, Client: client})
	})
}

// subagentWatchWriteWeb opens or ends the watch of one subagent page. It is a
// function, not a Manager method, to keep the Manager under its method limit.
func subagentWatchWriteWeb(mg *Manager, w http.ResponseWriter, r *http.Request) {
	var req subagent.WatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	wire.WriteJSON(w, openSubagentWatch(mg, req))
}

func openSubagentWatch(mg *Manager, req subagent.WatchRequest) wire.SessionResult {
	if req.Client == "" || req.Session == "" || req.CallID == "" {
		return Refusal(wire.ReasonSessionBadRequest, "a subagent watch needs a client, a session and a call")
	}
	key := [2]string{req.Session, req.CallID}
	if req.Closed {
		mg.subagentWatches.end(req.Client, key)
		return wire.SessionResult{}
	}
	dir, _, res, err := mg.SessionDir(req.Session)
	if res.Refused || err != nil {
		return res
	}
	if !mg.focus.connected(req.Client) {
		return Refusal(wire.ReasonSubagentWatchUnknownClient, "client "+req.Client+" has no open event stream")
	}
	watch, known := chat.WatchSubagent(mg.chat, req.Session, dir, req.CallID, func() {
		mg.events.PublishEvent(wire.Event{Type: wire.EventSessionSubagentChanged, Address: req.Session, Call: req.CallID, Client: req.Client})
	})
	if !known {
		return Refusal(wire.ReasonUnknownSubagent, "the session has no subagent call "+req.CallID)
	}
	if watch == nil {
		mg.subagentWatches.end(req.Client, key)
		return wire.SessionResult{}
	}
	mg.subagentWatches.put(req.Client, key, watch)
	// The stream may have closed while the watch started.
	if !mg.focus.connected(req.Client) {
		mg.subagentWatches.endClient(req.Client)
	}
	return wire.SessionResult{}
}
