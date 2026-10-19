package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
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

// runEndLine matches the closing line of a gx-design run, capturing the next
// phase.
var runEndLine = regexp.MustCompile(`(?m)^.*✅ [^:\n]+:.*Next: (\w+) —.*$`)

// designActionable is what a Design session's last assistant message asks of
// the person: the grilling question wins over the run-end line.
func designActionable(text, ticket string) (SessionActionable, bool) {
	if strings.Contains(text, grillingMarker) {
		return SessionActionable{Reason: grillingReason}, true
	}
	m := runEndLine.FindStringSubmatch(text)
	if m == nil {
		return SessionActionable{}, false
	}
	reason := strings.TrimSpace(m[0])
	epic := ticketEpic(ticket)
	switch m[1] {
	case "map", "review":
		return SessionActionable{Reason: reason, Action: []string{"/clear", "/gx-design " + epic}}, true
	case "implement":
		return SessionActionable{Reason: reason, Kind: SessionActionableQueue, Target: epicAddress(ticket) + "-impl"}, true
	}
	return SessionActionable{}, false
}

// ticketEpic is the epic of a ticket address like "project:epic" or
// "project:epic/03".
func ticketEpic(addr string) string {
	_, rest, _ := strings.Cut(addr, ":")
	epic, _, _ := strings.Cut(rest, "/")
	return epic
}

// epicAddress is the "project:epic" part of a ticket address, whatever project
// the session itself runs in.
func epicAddress(addr string) string {
	epicPart, _, _ := strings.Cut(addr, "/")
	return epicPart
}

// detectDesignActionable raises the grilling or run-end Actionable when a
// Design session's last assistant message holds its marker. Like an
// agent-raised one, the person's next input clears it. A harness without a
// claude transcript (codex) never matches.
func (s *Server) detectDesignActionable(dir, id, conversationID, ticket string) {
	projects, err := harness.ClaudeProjectsDir()
	if err != nil || conversationID == "" {
		return
	}
	paths, err := transcript.FindByIDIn(projects, conversationID)
	if err != nil || len(paths) != 1 {
		return
	}
	text, ok, err := transcript.LastAssistantText(paths[0])
	if err != nil || !ok {
		return
	}
	a, ok := designActionable(text, ticket)
	if !ok {
		return
	}
	raised := false
	s.updateSessionMetaOrWarn(dir, id, func(m *sessionMeta) bool {
		if m.Status.State != SessionIdle || m.Actionable.Reason != "" {
			return false
		}
		m.Actionable = a
		raised = true
		return true
	})
	if raised {
		s.triggerBanner(id, sound.Attention, a.Reason)
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
