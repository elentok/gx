package prompt

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/chatreader"
	"github.com/elentok/gx/grillround"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/sound"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/transcript"
	triagerec "github.com/elentok/gx/triage"
)

func (p *Prompt) actionableWrite(w http.ResponseWriter, r *http.Request) {
	var req wire.MarkActionableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := p.markSessionActionable(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, res)
}

// markSessionActionable saves an agent-raised Actionable, replacing any
// earlier one.
func (p *Prompt) markSessionActionable(req wire.MarkActionableRequest) (wire.SessionResult, error) {
	if req.Reason == "" {
		return sessions.Refusal(wire.ReasonSessionBadRequest, "an actionable needs a reason"), nil
	}
	dir, meta, res, err := p.sessions.SessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	if meta.Status.State == wire.SessionExited {
		return sessions.Refusal(wire.ReasonSessionNotLive, fmt.Sprintf("session %s has exited", req.ID)), nil
	}
	var info wire.SessionInfo
	if err := p.sessions.UpdateSessionMeta(dir, req.ID, func(m *sessions.Meta) bool {
		m.Actionable = wire.SessionActionable{Reason: req.Reason, Action: req.Action}
		info = m.Info()
		return true
	}); err != nil {
		return wire.SessionResult{}, err
	}
	p.sessions.TriggerBanner(req.ID, sound.Attention, req.Reason)
	return wire.SessionResult{Session: &info}, nil
}

const grillingReason = "Grilling: waiting for your answers"

// designActionable is what a Design session's last assistant message asks of
// the person: the grilling question wins over the run-end line. A message that
// ends on a prose question has no marker gx can show, but the session still
// waits on the answer, so it raises no Continue for auto-advance to send.
func designActionable(text string, ref chat.FlowRef, agent string, final bool) (wire.SessionActionable, bool) {
	if ref.Flow == tickets.FlowDesign && grillround.HasMarker(text) {
		return wire.SessionActionable{Reason: grillingReason, Grilling: true, Derived: true}, true
	}
	if strings.HasSuffix(strings.TrimSpace(text), "?") {
		return wire.SessionActionable{}, false
	}
	return chat.RunEndNext(text, ref, agent, final)
}

// StepFinal reports whether the flow is at a step with no Continue. A Design
// reads its map's progress, a Triage its project's live record.
func (p *Prompt) StepFinal(ref chat.FlowRef) bool {
	switch ref.Flow {
	case tickets.FlowDesign:
		return p.designFinal(ref.Key)
	case tickets.FlowTriage:
		return p.triageFinal(ref.Key)
	}
	return true
}

// triageFinal reads the project's live Triage record. A record that cannot be
// read is final: nothing auto-advances by accident.
func (p *Prompt) triageFinal(project string) bool {
	dir, err := p.projects.Dir(project)
	if err != nil {
		return true
	}
	rec, err := triagerec.NewStore(dir).LoadLive()
	if err != nil {
		p.log.Warn("triage step: read record", "project", project, "err", err)
		return true
	}
	return triagerec.Final(rec)
}

// designFinal reports whether the map behind a design ticket is at Branch or
// Done, where no step follows. A map that cannot be read is not final.
func (p *Prompt) designFinal(ticket string) bool {
	addr := tickets.EpicAddress(ticket)
	project, name, _ := strings.Cut(addr, ":")
	epics, err := p.projects.Epics(project)
	if err != nil {
		p.log.Warn("design step: read map", "ticket", ticket, "err", err)
		return false
	}
	for _, e := range epics {
		if e.Name == name {
			step := queue.MapProgress(e)
			return step == wire.ProgressBranch || step == wire.ProgressDone
		}
	}
	return false
}

// lastAssistantText reads the session's last assistant message from its claude
// transcript, or from its codex rollout.
func (p *Prompt) lastAssistantText(id string, meta sessions.Meta) (string, bool) {
	if meta.Harness == string(harness.Codex) {
		text, ok, err := p.projects.LastCodexText(meta.ConversationID)
		if err != nil {
			p.log.Warn("design actionable: read codex rollout", "session", id, "err", err)
		}
		return text, ok && err == nil
	}
	path, ok := p.sessionChat.ClaudeTranscript(id, meta.ChatMeta())
	if !ok {
		return "", false
	}
	text, ok, err := transcript.LastAssistantText(path, transcript.NoTextCap)
	if err != nil {
		p.log.Warn("design actionable: read transcript", "session", id, "err", err)
	}
	return text, ok && err == nil
}

// DetectDesignActionable recomputes the Actionable of a Design session from
// its last assistant message: the newer turn supersedes an agent-raised one
// and clears a derived one it no longer matches. A GxRaised one is never
// touched.
func (p *Prompt) DetectDesignActionable(dir, id string, design chat.DesignRef) {
	meta, err := sessions.ReadMeta(dir)
	if err != nil {
		meta = sessions.Meta{}
	}
	meta.ConversationID = design.Conversation
	if design.Flow.Flow == tickets.FlowTriage && p.onTriageIdle != nil {
		// After the Actionable is settled: the stop must not race the run-end line.
		defer p.onTriageIdle(design.Flow.Key)
	}
	text, ok := p.lastAssistantText(id, meta)
	if !ok {
		return
	}
	if design.Flow.Flow == tickets.FlowDesign {
		p.badRounds.FileBadRoundBlocks(id, design, text)
	}
	a, matched := designActionable(text, design.Flow, meta.Harness, p.StepFinal(design.Flow))
	outcome := actionableNone
	var refusal string
	p.sessions.UpdateSessionMetaOrWarn(dir, id, func(m *sessions.Meta) bool {
		var next wire.SessionActionable
		next, outcome = decideActionable(*m, a, matched, actionablePolicy{Live: design.Live, MaxAutoSends: MaxAutoSends})
		if outcome == actionableNone {
			return false
		}
		refusal = refusalReason(*m, a, design.Live)
		m.Actionable = next
		return true
	})
	step := design.Flow.Key
	switch outcome {
	case actionableAdvance:
		// A countdown that starts is silent: the person is told only if it does not send.
		if p.StartAutoAdvance(id) {
			p.logAutoAdvance(id, step, "taken", "ended-actionable", a.Reason)
			p.events.Publish(wire.EventSessionChanged, id)
			return
		}
		p.logAutoAdvance(id, step, "refused", "countdown-off-or-held", a.Reason)
		p.sessions.TriggerBanner(id, sound.Attention, a.Reason)
	case actionableCapped:
		p.logAutoAdvance(id, step, "refused", "budget", a.Reason)
		p.sessions.TriggerBanner(id, sound.Attention, fmt.Sprintf("Auto-advance stopped after %d runs in a row: %s", MaxAutoSends, a.Reason))
	case actionableRaised:
		p.logAutoAdvance(id, step, "refused", refusal, a.Reason)
		if a.Grilling {
			p.sessions.TriggerQuestionBanner(id, sound.Attention, a.Reason, designQuestion(p.grillingRounds(id, meta), text))
			return
		}
		p.sessions.TriggerBanner(id, sound.Attention, a.Reason)
	}
}

// designQuestion is the Grilling round a Design waits on: its number and the
// round's first question.
func designQuestion(round int, text string) sessions.DesignQuestion {
	q := sessions.DesignQuestion{Round: round}
	if cards := chat.RoundCards(text); len(cards) > 0 {
		q.Text = firstLine(cards[0].Body)
		if q.Text == "" {
			q.Text = cards[0].Title
		}
	}
	return q
}

// grillingRounds counts the Grilling rounds in the session's conversation with
// the counter the conversation record uses, summaries not counted. It reads the
// transcript now: the record is saved only when the conversation ends. 0 when
// the count cannot be read.
func (p *Prompt) grillingRounds(id string, meta sessions.Meta) int {
	path, kind, ok := p.sessionChat.Transcript(id, meta.ChatMeta())
	if !ok {
		return 0
	}
	res, ok, err := chatreader.ReadFile(kind, path, "")
	if err != nil || !ok {
		p.log.Warn("design question: read conversation", "session", id, "err", err)
		return 0
	}
	rounds := 0
	for _, it := range grillround.Count(res.Messages) {
		if it.Questions > 0 {
			rounds++
		}
	}
	return rounds
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(line)
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

// decideActionable is the pure decision behind DetectDesignActionable: the
// Actionable cur's meta should hold after a turn that matched a (or did not),
// and what to do about it. A GxRaised one, or a session not waiting for the
// person, is never touched.
func decideActionable(cur sessions.Meta, a wire.SessionActionable, matched bool, p actionablePolicy) (wire.SessionActionable, actionableOutcome) {
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
	if !p.Live || !cur.CanAutoAdvance() {
		return a, actionableRaised
	}
	if cur.AutoSends >= p.MaxAutoSends {
		return a, actionableCapped
	}
	return a, actionableAdvance
}

// refusalReason is why a raised Actionable on cur's meta does not start an
// auto-advance countdown, in the order decideActionable checks.
func refusalReason(cur sessions.Meta, a wire.SessionActionable, live bool) string {
	switch {
	case a.Grilling:
		return "open-question"
	case cur.AutoAdvanceOff:
		return "switch-off"
	case len(a.Action) == 0:
		return "no-action"
	case !live:
		return "not-live"
	}
	return "not-advanceable"
}

// logAutoAdvance writes the one line per auto-advance decision, taken or
// refused, so "why did it not advance" is as answerable as "why did it".
func (p *Prompt) logAutoAdvance(id, ticket, decision, reason, runEnd string) {
	p.log.Info("auto-advance decision", "session", id, "ticket", ticket, "decision", decision, "reason", reason, "run_end", runEnd)
}

// RaiseGxActionable saves an Actionable gx raises itself, replacing any
// earlier one. The person's input does not clear it.
func (p *Prompt) RaiseGxActionable(dir, reason string, action []string) {
	p.raiseActionable(dir, wire.SessionActionable{Reason: reason, Action: action, GxRaised: true})
}

// raiseActionable is RaiseGxActionable for an Actionable with more than a
// reason and prompts.
func (p *Prompt) raiseActionable(dir string, a wire.SessionActionable) {
	id := filepath.Base(dir)
	reason := a.Reason
	raised := false
	p.sessions.UpdateSessionMetaOrWarn(dir, id, func(m *sessions.Meta) bool {
		if m.Status.State == wire.SessionExited {
			return false
		}
		m.Actionable = a
		raised = true
		return true
	})
	if raised {
		p.sessions.TriggerBanner(id, sound.Attention, reason)
	}
}
