package sessions

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/elentok/gx/server/wire"
)

// askKey identifies a held question: one AskUserQuestion call of one session.
type askKey struct{ session, toolUseID string }

// askHolds are the AskUserQuestion calls a Claude hook is holding, each waiting
// for a result. The zero value is ready to use.
type askHolds struct {
	mu   sync.Mutex
	held map[askKey]*heldAsk
	// released are the dialogs handed back to the terminal, with their question
	// texts; a hook that asks again for one gets a release at once, and the
	// screen fallback card skips a dialog showing one of those questions.
	released map[askKey][]string
}

type heldAsk struct {
	req    wire.AskHookRequest
	result chan wire.AskHookResult // buffered: resolving never blocks
}

// HoldAsk registers the question and waits for ResolveAsk. When ctx ends first
// (the hook went away) it returns a release, which leaves Claude's own dialog.
func (mg *Manager) HoldAsk(ctx context.Context, req wire.AskHookRequest) wire.AskHookResult {
	key := askKey{req.Session, req.ToolUseID}
	h := &heldAsk{req: req, result: make(chan wire.AskHookResult, 1)}
	a := &mg.asks
	a.mu.Lock()
	if _, ok := a.released[key]; ok {
		a.mu.Unlock()
		return wire.AskHookResult{Decision: wire.AskHookRelease}
	}
	if a.held == nil {
		a.held = map[askKey]*heldAsk{}
	}
	if old, ok := a.held[key]; ok {
		old.result <- wire.AskHookResult{Decision: wire.AskHookRelease}
	}
	a.held[key] = h
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		if a.held[key] == h {
			delete(a.held, key)
		}
		a.mu.Unlock()
	}()
	select {
	case res := <-h.result:
		return res
	case <-ctx.Done():
		return wire.AskHookResult{Decision: wire.AskHookRelease}
	}
}

// HeldAsks lists a session's held questions.
func (mg *Manager) HeldAsks(session string) []wire.AskHookRequest {
	a := &mg.asks
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []wire.AskHookRequest
	for k, h := range a.held {
		if k.session == session {
			out = append(out, h.req)
		}
	}
	slices.SortFunc(out, func(x, y wire.AskHookRequest) int { return strings.Compare(x.ToolUseID, y.ToolUseID) })
	return out
}

// ForgetReleasedAsks drops a session's released dialogs: the session ended or
// a new blocker started, so a later dialog with the same question is a new one.
func (mg *Manager) ForgetReleasedAsks(session string) {
	a := &mg.asks
	a.mu.Lock()
	defer a.mu.Unlock()
	for k := range a.released {
		if k.session == session {
			delete(a.released, k)
		}
	}
}

// ResolveAsk gives a held question its result and reports whether one was held.
func (mg *Manager) ResolveAsk(session, toolUseID string, res wire.AskHookResult) bool {
	a := &mg.asks
	a.mu.Lock()
	defer a.mu.Unlock()
	h, ok := a.held[askKey{session, toolUseID}]
	if !ok {
		return false
	}
	select {
	case h.result <- res:
	default:
		return false
	}
	delete(a.held, askKey{session, toolUseID})
	return true
}

// ReleaseAsk releases a held question to Claude's terminal dialog and
// remembers its id.
func (mg *Manager) ReleaseAsk(session, toolUseID string) bool {
	a := &mg.asks
	a.mu.Lock()
	defer a.mu.Unlock()
	key := askKey{session, toolUseID}
	h, ok := a.held[key]
	if !ok {
		return false
	}
	select {
	case h.result <- wire.AskHookResult{Decision: wire.AskHookRelease}:
	default:
		return false
	}
	delete(a.held, key)
	var questions []string
	for _, q := range h.req.Questions {
		questions = append(questions, q.Question)
	}
	if a.released == nil {
		a.released = map[askKey][]string{}
	}
	a.released[key] = questions
	return true
}

// AskReleased reports whether the session handed a dialog showing question
// back to the terminal, so the screen fallback must not list it again.
func (mg *Manager) AskReleased(session, question string) bool {
	a := &mg.asks
	a.mu.Lock()
	defer a.mu.Unlock()
	for k, qs := range a.released {
		if k.session == session && slices.Contains(qs, question) {
			return true
		}
	}
	return false
}

// AskHoldWrite is POST /v1/sessions/ask-hold: the hook's call, which blocks
// until the question has a result.
func (mg *Manager) AskHoldWrite(w http.ResponseWriter, r *http.Request) {
	var req wire.AskHookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Session == "" || req.ToolUseID == "" {
		http.Error(w, "ask-hold needs a session and a tool_use_id", http.StatusBadRequest)
		return
	}
	wire.WriteJSON(w, mg.HoldAsk(r.Context(), req))
}
