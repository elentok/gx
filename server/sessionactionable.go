package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/sound"
	"github.com/elentok/gx/transcript"
)

func (s *Server) sessionActionableWrite(w http.ResponseWriter, r *http.Request) {
	var req MarkActionableRequest
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
func (s *Server) markSessionActionable(req MarkActionableRequest) (SessionResult, error) {
	if req.Reason == "" {
		return sessionRefusal(ReasonSessionBadRequest, "an actionable needs a reason"), nil
	}
	dir, meta, res, err := s.sessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if meta.Status.State == SessionExited {
		return sessionRefusal(ReasonSessionNotLive, fmt.Sprintf("session %s has exited", req.ID)), nil
	}
	var info SessionInfo
	if err := s.updateSessionMeta(dir, req.ID, func(m *sessionMeta) bool {
		m.Actionable = SessionActionable{Reason: req.Reason, Action: req.Action}
		info = m.info()
		return true
	}); err != nil {
		return SessionResult{}, err
	}
	s.triggerBanner(req.ID, sound.Attention, req.Reason)
	return SessionResult{Session: &info}, nil
}

// grillingMarker opens each question of a gx-design grilling round.
const grillingMarker = "❓ **Q"

const grillingReason = "Grilling: waiting for your answers"

// detectGrilling raises the grilling Actionable when a Design session's last
// assistant message holds a question marker. Like an agent-raised one, the
// person's next input clears it. A harness without a claude transcript
// (codex) never matches.
func (s *Server) detectGrilling(dir, id, conversationID string) {
	projects, err := harness.ClaudeProjectsDir()
	if err != nil || conversationID == "" {
		return
	}
	paths, err := transcript.FindByIDIn(projects, conversationID)
	if err != nil || len(paths) != 1 {
		return
	}
	text, ok, err := transcript.LastAssistantText(paths[0])
	if err != nil || !ok || !strings.Contains(text, grillingMarker) {
		return
	}
	raised := false
	s.updateSessionMetaOrWarn(dir, id, func(m *sessionMeta) bool {
		if m.Status.State != SessionIdle || m.Actionable.Reason != "" {
			return false
		}
		m.Actionable = SessionActionable{Reason: grillingReason}
		raised = true
		return true
	})
	if raised {
		s.triggerBanner(id, sound.Attention, grillingReason)
	}
}

// raiseGxActionable saves an Actionable gx raises itself, replacing any
// earlier one. The person's input does not clear it.
func (s *Server) raiseGxActionable(dir, reason string, action []string) {
	id := filepath.Base(dir)
	raised := false
	s.updateSessionMetaOrWarn(dir, id, func(m *sessionMeta) bool {
		if m.Status.State == SessionExited {
			return false
		}
		m.Actionable = SessionActionable{Reason: reason, Action: action, GxRaised: true}
		raised = true
		return true
	})
	if raised {
		s.triggerBanner(id, sound.Attention, reason)
	}
}
