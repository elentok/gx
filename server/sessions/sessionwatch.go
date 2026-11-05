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

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner"
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
		if time.Now().Before(crashUntil) {
			exit = wire.SessionExitCrashed
		}
		update(func(m *Meta) bool { return mg.recordSessionExit(m, exit) })
	}
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
	seenBusy, sawIdle := false, !reconnect
	for {
		msg, err := stream.Next()
		if err != nil {
			if !mg.Watches.IsStopped() {
				recordExit(savedSessionExit(dir))
			}
			return
		}
		switch msg.Type {
		case nativerunner.StreamStatus:
			st := SessionStatusOf(msg.Status)
			var prompt, signal string
			var design chat.DesignRef
			idleEdge := false
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
					m.Status, m.StatusChangedAt = st, time.Now()
					changed = true
					if st.Promptable() {
						m.ReportedStep, m.ReportedTitle = "", ""
					}
					if st.Promptable() && !prev.Promptable() {
						idleEdge = true
						if m.Design {
							design = chat.DesignRef{Conversation: m.ConversationID, Ticket: m.Ticket, Live: live}
						}
					}
				}
				if msg.ConversationID != "" && msg.ConversationID != m.ConversationID {
					mg.saveConversationTotals(*m, m.ConversationID)
					harness.CarryEditorOverride(m.ConversationID, msg.ConversationID)
					m.ConversationID = msg.ConversationID
					mg.recordConversation(*m)
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
			poll.SetActive(st.State == wire.SessionStarting || st.State == wire.SessionWorking)
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
			if prompt != "" {
				first.Start(signal, prompt)
			}
		case nativerunner.StreamInput:
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
		case nativerunner.StreamExit:
			recordExit(sessionExitOf(msg.ExitCode))
			return
		}
	}
}

// typeSessionPrompt sends Enter on its own, so the harness reads the prompt
// as typed rather than as a paste ending in a newline.
func TypeSessionPrompt(dir, prompt string) error {
	c, err := nativerunner.DialHolder(dir)
	if err == nil {
		err = c.TypeText(prompt)
	}
	if err == nil {
		err = c.SendKeys("\r")
	}
	return err
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
	data, err := os.ReadFile(filepath.Join(dir, nativerunner.ExitCodeFile))
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
func (mg *Manager) openSessionWatch(dir string) (*nativerunner.HolderWatch, error) {
	c, err := mg.dialNewHolder(dir)
	if errors.Is(err, errHolderExited) {
		return nil, errors.New("holder exited before its watch opened")
	}
	if err != nil {
		return nil, err
	}
	if !c.Supports(nativerunner.OpWatch) {
		return nil, errors.New("holder does not serve watch")
	}
	return c.Watch()
}

var errHolderExited = errors.New("holder exited")

// dialNewHolder waits for a just-started holder's socket, until the holder
// saves an exit, holderDialTimeout passes or the server stops.
func (mg *Manager) dialNewHolder(dir string) (*nativerunner.HolderClient, error) {
	deadline := time.Now().Add(holderDialTimeout)
	for {
		c, err := nativerunner.DialHolder(dir)
		if err == nil {
			return c, nil
		}
		if _, statErr := os.Stat(filepath.Join(dir, nativerunner.ExitCodeFile)); statErr == nil {
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
		if exit != wire.SessionExitClean || !mg.designDone(*m) {
			mg.TriggerBanner(m.ID, sound.Done, "")
		}
	}
	mg.saveConversationTotals(*m, m.ConversationID)
	m.Status, m.StatusChangedAt, m.Exit = wire.SessionStatus{State: wire.SessionExited}, time.Now(), exit
	m.RestoreCrashed = exit == wire.SessionExitCrashed && !m.RestoredAt.IsZero() && time.Since(m.RestoredAt) < chat.RestoreCrashWindow
	m.FirstPrompt = ""
	m.Actionable = wire.SessionActionable{}
	return true
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
