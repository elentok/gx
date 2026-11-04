package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/grillround"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/sound"
	"github.com/elentok/gx/transcript"
)

func (s *Server) sessionActionableWrite(w http.ResponseWriter, r *http.Request) {
	var req wire.MarkActionableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := s.markSessionActionable(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
}

// markSessionActionable saves an agent-raised Actionable, replacing any
// earlier one.
func (s *Server) markSessionActionable(req wire.MarkActionableRequest) (wire.SessionResult, error) {
	if req.Reason == "" {
		return sessionRefusal(wire.ReasonSessionBadRequest, "an actionable needs a reason"), nil
	}
	dir, meta, res, err := s.sessions.SessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if meta.Status.State == wire.SessionExited {
		return sessionRefusal(wire.ReasonSessionNotLive, fmt.Sprintf("session %s has exited", req.ID)), nil
	}
	var info wire.SessionInfo
	if err := s.sessions.updateSessionMeta(dir, req.ID, func(m *sessionMeta) bool {
		m.Actionable = wire.SessionActionable{Reason: req.Reason, Action: req.Action}
		info = m.info()
		return true
	}); err != nil {
		return wire.SessionResult{}, err
	}
	s.triggerBanner(req.ID, sound.Attention, req.Reason)
	return wire.SessionResult{Session: &info}, nil
}

const grillingReason = "Grilling: waiting for your answers"

// designActionable is what a Design session's last assistant message asks of
// the person: the grilling question wins over the run-end line.
func designActionable(text, ticket, agent string, final bool) (wire.SessionActionable, bool) {
	if grillround.HasMarker(text) {
		return wire.SessionActionable{Reason: grillingReason, Grilling: true, Derived: true}, true
	}
	return chat.RunEndNext(text, ticket, agent, final)
}

// designFinal reports whether the map behind a design ticket is at Branch or
// Done, where no step follows. A map that cannot be read is not final.
func (s *Server) designFinal(ticket string) bool {
	addr := epicAddress(ticket)
	project, name, _ := strings.Cut(addr, ":")
	epics, err := s.projects.Epics(project)
	if err != nil {
		s.log.Warn("design step: read map", "ticket", ticket, "err", err)
		return false
	}
	for _, e := range epics {
		if e.Name == name {
			step := mapProgress(e)
			return step == wire.ProgressBranch || step == wire.ProgressDone
		}
	}
	return false
}

// epicAddress is the "project:epic" part of a ticket address, whatever project
// the session itself runs in.
func epicAddress(addr string) string {
	epicPart, _, _ := strings.Cut(addr, "/")
	return epicPart
}

// designRef names the conversation and ticket of a Design session that just
// went idle. Live marks a turn that ended while this server watched it, the
// only kind that auto-advances.
type designRef struct {
	Conversation, Ticket string
	Live                 bool
}

// lastAssistantText reads the session's last assistant message from its claude
// transcript, or from its codex rollout.
func (s *Server) lastAssistantText(id string, meta sessionMeta) (string, bool) {
	if meta.Harness == string(harness.Codex) {
		text, ok, err := s.projects.LastCodexText(meta.ConversationID)
		if err != nil {
			s.log.Warn("design actionable: read codex rollout", "session", id, "err", err)
		}
		return text, ok && err == nil
	}
	path, ok := s.sessionChat.ClaudeTranscript(id, chatMeta(meta))
	if !ok {
		return "", false
	}
	text, ok, err := transcript.LastAssistantText(path, transcript.NoTextCap)
	if err != nil {
		s.log.Warn("design actionable: read transcript", "session", id, "err", err)
	}
	return text, ok && err == nil
}

// detectDesignActionable recomputes the Actionable of a Design session from
// its last assistant message: the newer turn supersedes an agent-raised one
// and clears a derived one it no longer matches. A GxRaised one is never
// touched.
func (s *Server) detectDesignActionable(dir, id string, design designRef) {
	meta, err := readSessionMeta(dir)
	if err != nil {
		meta = sessionMeta{}
	}
	meta.ConversationID = design.Conversation
	text, ok := s.lastAssistantText(id, meta)
	if !ok {
		return
	}
	s.fileBadRoundBlocks(id, design, text)
	a, matched := designActionable(text, design.Ticket, meta.Harness, s.designFinal(design.Ticket))
	outcome := actionableNone
	s.sessions.updateSessionMetaOrWarn(dir, id, func(m *sessionMeta) bool {
		var next wire.SessionActionable
		next, outcome = decideActionable(*m, a, matched, actionablePolicy{Live: design.Live, MaxAutoSends: maxAutoSends})
		if outcome == actionableNone {
			return false
		}
		m.Actionable = next
		return true
	})
	switch outcome {
	case actionableAdvance:
		// A countdown that starts is silent: the person is told only if it does not send.
		if s.startAutoAdvance(id) {
			s.events.Publish(wire.EventSessionChanged, id)
			return
		}
		s.triggerBanner(id, sound.Attention, a.Reason)
	case actionableCapped:
		s.triggerBanner(id, sound.Attention, fmt.Sprintf("Auto-advance stopped after %d runs in a row: %s", maxAutoSends, a.Reason))
	case actionableRaised:
		s.triggerBanner(id, sound.Attention, a.Reason)
	}
}

// actionableOutcome is what decideActionable did to a session's Actionable.
type actionableOutcome int

const (
	// actionableNone leaves the Actionable as it was.
	actionableNone actionableOutcome = iota
	// actionableCleared drops a derived Actionable the newer turn no longer matches.
	actionableCleared
	// actionableRaised saves a new Actionable the person is told about.
	actionableRaised
	// actionableAdvance saves it and starts the auto-advance countdown.
	actionableAdvance
	// actionableCapped saves it, but the auto-send cap is reached.
	actionableCapped
)

// actionablePolicy is what decideActionable reads besides the meta: whether the
// turn ended while this server watched it, and the auto-send cap.
type actionablePolicy struct {
	Live         bool
	MaxAutoSends int
}

// decideActionable is the pure decision behind detectDesignActionable: the
// Actionable cur's meta should hold after a turn that matched a (or did not),
// and what to do about it. A GxRaised one, or a session not waiting for the
// person, is never touched.
func decideActionable(cur sessionMeta, a wire.SessionActionable, matched bool, p actionablePolicy) (wire.SessionActionable, actionableOutcome) {
	old := cur.Actionable
	if !cur.Status.Promptable() || old.GxRaised {
		return old, actionableNone
	}
	if !matched {
		if !old.Derived {
			return old, actionableNone
		}
		return wire.SessionActionable{}, actionableCleared
	}
	if old.Derived && old.Reason == a.Reason && old.Grilling == a.Grilling {
		return old, actionableNone
	}
	cur.Actionable = a
	if !p.Live || !cur.canAutoAdvance() {
		return a, actionableRaised
	}
	if cur.AutoSends >= p.MaxAutoSends {
		return a, actionableCapped
	}
	return a, actionableAdvance
}

// raiseGxActionable saves an Actionable gx raises itself, replacing any
// earlier one. The person's input does not clear it.
func (s *Server) raiseGxActionable(dir, reason string, action []string) {
	id := filepath.Base(dir)
	raised := false
	s.sessions.updateSessionMetaOrWarn(dir, id, func(m *sessionMeta) bool {
		if m.Status.State == wire.SessionExited {
			return false
		}
		m.Actionable = wire.SessionActionable{Reason: reason, Action: action, GxRaised: true}
		raised = true
		return true
	})
	if raised {
		s.triggerBanner(id, sound.Attention, reason)
	}
}
