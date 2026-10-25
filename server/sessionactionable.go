package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

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

func (s *Server) sessionStepWrite(w http.ResponseWriter, r *http.Request) {
	var req ReportStepRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := s.reportSessionStep(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
}

// reportSessionStep saves the Design step an agent says it is running,
// replacing any earlier report.
func (s *Server) reportSessionStep(req ReportStepRequest) (SessionResult, error) {
	if req.Step == "" {
		return sessionRefusal(ReasonSessionBadRequest, "a step report needs a step"), nil
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
		m.ReportedStep, m.ReportedTitle = req.Step, req.Title
		info = m.info()
		return true
	}); err != nil {
		return SessionResult{}, err
	}
	return SessionResult{Session: &info}, nil
}

// grillingMarker opens each question of a gx-design grilling round. It counts
// only at the start of a line: a question may quote it inline, in backticks.
const grillingMarker = "❓ **Q"

var grillingMarkerRE = regexp.MustCompile(`(?m)^[ \t]*` + regexp.QuoteMeta(grillingMarker))

// hasGrillingMarker reports whether text opens a grilling question.
func hasGrillingMarker(text string) bool { return grillingMarkerRE.MatchString(text) }

const grillingReason = "Grilling: waiting for your answers"

// runEndLine matches the closing line of a gx-design run. It only marks that a
// run ended: the text after "Next:" is the agent's own, and nothing reads a
// phase from it.
var runEndLine = regexp.MustCompile(`(?m)^.*✅ [^:\n]+:.*Next:.*$`)

// designActionable is what a Design session's last assistant message asks of
// the person: the grilling question wins over the run-end line.
func designActionable(text, ticket string, final bool) (SessionActionable, bool) {
	if hasGrillingMarker(text) {
		return SessionActionable{Reason: grillingReason, Grilling: true, Derived: true}, true
	}
	return runEndNext(text, ticket, final)
}

// runEndNext is the Actionable a gx-design run-end line raises: Continue in a
// fresh conversation, or just the line when the design is at its final step.
func runEndNext(text, ticket string, final bool) (SessionActionable, bool) {
	line := runEndLine.FindString(text)
	if line == "" {
		return SessionActionable{}, false
	}
	a := SessionActionable{Reason: strings.TrimSpace(line), Derived: true}
	if !final {
		a.Action = designContinueAction(ticket)
	}
	return a, true
}

// designFinal reports whether the map behind a design ticket is at Branch or
// Done, where no step follows. A map that cannot be read is not final.
func (s *Server) designFinal(ticket string) bool {
	addr := epicAddress(ticket)
	project, name, _ := strings.Cut(addr, ":")
	epics, err := s.projectEpics(project)
	if err != nil {
		s.log.Warn("design step: read map", "ticket", ticket, "err", err)
		return false
	}
	for _, e := range epics {
		if e.Name == name {
			step := mapProgress(e)
			return step == ProgressBranch || step == ProgressDone
		}
	}
	return false
}

// designContinueAction starts the next gx-design phase in a fresh conversation.
func designContinueAction(ticket string) []string {
	return []string{"/clear", "/gx-design " + ticketEpic(ticket)}
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

// designRef names the conversation and ticket of a Design session that just
// went idle. Live marks a turn that ended while this server watched it, the
// only kind that auto-advances.
type designRef struct {
	Conversation, Ticket string
	Live                 bool
}

// claudeTranscript finds the claude transcript of a session's conversation. A
// missing transcript (a codex session, or one not written yet) stays silent;
// real errors and an ambiguous id are logged.
func (s *Server) claudeTranscript(id, conversationID string) (string, bool) {
	if conversationID == "" {
		return "", false
	}
	return s.transcripts.resolve(conversationID, time.Now(), s.cfg.TranscriptMissTTL, func() (string, bool) {
		projects, err := harness.ClaudeProjectsDir()
		if err != nil {
			s.log.Warn("claude projects dir", "session", id, "err", err)
			return "", false
		}
		paths, err := transcript.FindByIDIn(projects, conversationID)
		if err != nil {
			s.log.Warn("find transcript", "session", id, "err", err)
			return "", false
		}
		if len(paths) > 1 {
			s.log.Warn("several transcripts", "session", id, "conversation", conversationID, "count", len(paths))
		}
		if len(paths) != 1 {
			return "", false
		}
		return paths[0], true
	})
}

// lastAssistantText reads the session's last assistant message from its claude
// transcript.
func (s *Server) lastAssistantText(id, conversationID string) (string, bool) {
	path, ok := s.claudeTranscript(id, conversationID)
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
// touched. A harness without a claude transcript (codex) never matches.
func (s *Server) detectDesignActionable(dir, id string, design designRef) {
	text, ok := s.lastAssistantText(id, design.Conversation)
	if !ok {
		return
	}
	s.fileBadRoundBlocks(id, design, text)
	a, matched := designActionable(text, design.Ticket, s.designFinal(design.Ticket))
	outcome := actionableNone
	s.updateSessionMetaOrWarn(dir, id, func(m *sessionMeta) bool {
		var next SessionActionable
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
			s.events.publish(EventSessionChanged, id)
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
func decideActionable(cur sessionMeta, a SessionActionable, matched bool, p actionablePolicy) (SessionActionable, actionableOutcome) {
	old := cur.Actionable
	if !cur.Status.Promptable() || old.GxRaised {
		return old, actionableNone
	}
	if !matched {
		if !old.Derived {
			return old, actionableNone
		}
		return SessionActionable{}, actionableCleared
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
