package server

import (
	"net/http"
	"time"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// actionInfo looks up each action's label and tooltip.
func actionInfo(actions []string) map[string]wire.ActionInfo {
	out := make(map[string]wire.ActionInfo, len(actions))
	for _, a := range actions {
		if v, ok := schema.VerbOfWord(a); ok {
			out[a] = wire.ActionInfo{Label: v.Label(), Tooltip: v.Tooltip()}
		}
	}
	return out
}

const permissionHint = "Approve it in the TUI"

func (s *Server) detail(w http.ResponseWriter, r *http.Request) {
	address := r.URL.Query().Get("address")
	// A malformed address is a ticket nobody has, the same as an unknown one.
	if _, err := tickets.ParseAddress(address, tickets.AddressContext{}); err != nil {
		http.Error(w, "no ticket "+address, http.StatusNotFound)
		return
	}
	ref, ok, err := s.FindTicket(address)
	switch {
	case err != nil:
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	case !ok:
		http.Error(w, "no ticket "+r.URL.Query().Get("address"), http.StatusNotFound)
		return
	}
	d, err := s.ticketDetail(ref)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, d)
}

func (s *Server) ticketDetail(ref queue.TicketRef) (wire.TicketDetail, error) {
	t, err := schema.ParseTicket(ref.Ticket.Path)
	if err != nil {
		return brokenDetail(ref, err), nil
	}
	evs, err := tickets.ReadLog(ref.Ticket.Path)
	if err != nil {
		return wire.TicketDetail{}, err
	}
	d := newTicketDetail(ref)
	fillHeader(&d, ref, t)
	if t.Status == schema.StatusClaimed {
		d.WaitingLabel = s.CheckWaitingLabel(ref.Addr.String())
	}
	if t.LongTicketTurns > 0 {
		d.LongTicketTurns, d.LongTicketThreshold = t.LongTicketTurns, s.settings.Load().LongTicketTurns
	}
	if !ref.Epic.IsMap {
		d.Stats = s.detailStats(ref)
		_, d.HasScreen = s.sessionChat.TicketPtyDir(ref.Addr)
		var writtenAt time.Time
		if t.Status == schema.StatusCancelled {
			writtenAt = ticketWrittenAt(ref.Ticket.Path)
		}
		if d.Runs, err = s.ticketRuns(ref, t, evs, writtenAt); err != nil {
			return wire.TicketDetail{}, err
		}
	}
	epics, err := s.projects.Epics(ref.Addr.Project)
	if err != nil {
		return wire.TicketDetail{}, err
	}
	d.Related = relatedTickets(ref, epics)
	// A blocked-pane pause keeps its agent's prompt, and Approve ends that pause.
	if t.Status == schema.StatusClaimed || t.PauseReason == schema.PauseBlockedPane {
		if d.Permission, err = s.permissionCard(ref.Addr); err != nil {
			return wire.TicketDetail{}, err
		}
	}
	d.State = detailState(evs, t)
	if rec := tickets.RecommendationOf(evs); rec != nil && t.PausedClass() != "" {
		d.Recommendation = &wire.DetailRecommendation{Verb: rec.Choice, Diagnosis: rec.Text}
	}
	rounds := tickets.Rounds(evs)
	if d.Actions, err = s.detailFor(ref.Addr.String(), t, rounds, d.Permission); err != nil {
		return wire.TicketDetail{}, err
	}
	d.ActionInfo = actionInfo(d.Actions)
	if len(rounds) == 0 {
		return d, nil
	}
	last := &rounds[len(rounds)-1]
	for _, r := range rounds[:len(rounds)-1] {
		d.Rounds = append(d.Rounds, detailRound(r, false))
	}
	latest := detailRound(*last, true)
	if at := tickets.LastPrompt(evs, last.Question.Seq); !at.IsZero() && last.End == nil {
		latest.PromptedAt = &at
		latest.Prompt = "Prompt sent to the running agent at " + at.Local().Format("15:04")
	}
	d.Latest = &latest
	d.Answered = t.Status == schema.StatusOpen && latest.Outcome == wire.RoundAnswered
	d.Chat = roundChat(latest)
	if t.PausedClass() == schema.PauseClassAnswer && t.PauseReason == schema.PauseAmbiguousBase {
		d.Bases = s.ambiguousBlockers(ref.Addr)
	}
	return d, nil
}

// detailFor is what a person may do with a parsed ticket now, given its pause
// rounds. The ticket detail and ticketRecommend both read their actions from here.
func (s *Server) detailFor(addr string, t schema.Ticket, rounds []tickets.PauseRound, card *wire.PermissionCard) ([]string, error) {
	var last *tickets.PauseRound
	if len(rounds) > 0 {
		last = &rounds[len(rounds)-1]
	}
	gone := false
	if t.PausedClass() != "" {
		var err error
		if gone, err = s.agentGone(addr); err != nil {
			return nil, err
		}
	}
	return detailActions(t, last, gone, card), nil
}

// brokenDetail is the small payload of a file that fails to parse: nothing the
// body or the log could add.
func brokenDetail(ref queue.TicketRef, parseErr error) wire.TicketDetail {
	d := newTicketDetail(ref)
	d.Status = tickets.StatusError.Word()
	d.ErrorReason = "ticket file format is invalid"
	d.ParseError = parseErr.Error()
	d.Path = ref.Ticket.Path
	return d
}

// newTicketDetail is the payload every ticket starts from: what the address
// alone gives, with every list empty rather than null.
func newTicketDetail(ref queue.TicketRef) wire.TicketDetail {
	return wire.TicketDetail{
		Address: ref.Addr.String(), Title: ref.Ticket.Title, Epic: queue.RootOf(ref.Addr).String(), MapEpic: ref.Epic.IsMap,
		Related: []wire.RelatedTicket{}, Rounds: []wire.DetailRound{}, Chat: []wire.ChatMessage{}, Actions: []string{}, Runs: []wire.DetailRun{},
	}
}

// fillHeader sets the fields of a ticket that parsed: its status, pause, body
// and, for an Error ticket, the reason.
func fillHeader(d *wire.TicketDetail, ref queue.TicketRef, t schema.Ticket) {
	d.Status = string(storedStatus(t))
	d.PauseReason = string(t.PauseReason)
	d.PauseLabel = t.PauseReason.Label()
	if t.PauseReason == schema.PauseModelMismatch {
		d.ModelRan, d.ModelExpected = t.ModelRan, t.ModelExpected
	}
	d.Type = ref.Ticket.Type
	d.Body = ref.Ticket.Body
	if ref.Epic.RenderedStatus(ref.Ticket) == tickets.StatusError {
		d.Status = tickets.StatusError.Word()
		d.ErrorReason = tickets.PauseDetail(ref.Epic, ref.Ticket)
		d.Path = ref.Ticket.Path
	}
	if t.PausedClass() != "" {
		d.PauseText = schema.Section(ref.Ticket.Body, "Paused")
	}
}

// permissionCard is the pending prompt of addr's live agent, or nil. It reads
// the runner's status, so it runs only when a detail or a snapshot is built.
// A prompt the web cannot approve (a herdr-hosted agent, an unnamed dialog, a
// tool that needs a real answer) gets a card with a hint and no request ID.
func (s *Server) permissionCard(addr tickets.Address) (*wire.PermissionCard, error) {
	runner, sess, ok, err := s.harness.LiveSession(addr)
	if err != nil || !ok {
		return nil, err
	}
	st, err := runner.Status(sess)
	if err != nil || st.State != agentrunner.StateBlocked {
		return nil, nil
	}
	p := st.Pending
	if p == nil {
		return &wire.PermissionCard{Description: st.BlockedReason, Hint: permissionHint}, nil
	}
	card := &wire.PermissionCard{RequestID: p.ID, Tool: p.Tool, Input: string(p.Input), Description: p.Description}
	if p.NotApprovable || runner.Name() == string(agentrunner.ChoiceHerdr) {
		card.RequestID, card.Hint = "", permissionHint
	}
	return card, nil
}

// detailActions is what a person may do with the ticket right now. Approve
// joins when the permission card can be approved from here.
func detailActions(t schema.Ticket, latest *tickets.PauseRound, agentGone bool, card *wire.PermissionCard) []string {
	actions := []string{}
	if card != nil && card.RequestID != "" {
		actions = append(actions, wire.ActionApprove)
	}
	if t.FinishedCommitless() {
		actions = append(actions, wire.ActionMerge)
	}
	if t.PausedClass() == "" {
		return actions
	}
	row, ok := t.EffectiveReason().Row()
	if !ok {
		return actions
	}
	approvable := len(actions) > 0
	for _, v := range row.Verbs {
		switch {
		case v == schema.VerbAllowDeny:
			// Approve is added above, from the live card.
		case v == schema.VerbAnswer && (latest == nil || latest.End != nil):
		case v == schema.VerbResume && t.PauseReason == schema.PauseSelfReported && !agentGone:
		case v == schema.VerbRetry && t.PauseReason == schema.PauseBlockedPane && approvable:
		default:
			actions = append(actions, v.Word())
		}
	}
	return actions
}

// detailRound reads a round's question leniently: a bad block is free text.
func detailRound(r tickets.PauseRound, full bool) wire.DetailRound {
	out := wire.DetailRound{Seq: r.Question.Seq, By: r.Question.By, Summary: r.Question.Summary, Outcome: wire.RoundOpen}
	if r.End != nil {
		out.Outcome = wire.RoundClosed
		if r.End.Type == tickets.EventPauseAnswer {
			out.Outcome = wire.RoundAnswered
		}
	}
	if !full {
		return out
	}
	q := tickets.QuestionOfEvent(r.Question)
	out.Kind, out.Text, out.Block, out.Handoff = q.Kind, q.Detail, r.Question.Block, q.Handoff
	out.Recommended, out.Yes, out.No = q.Recommended, q.Yes, q.No
	for _, o := range q.Options {
		out.Options = append(out.Options, wire.DetailOption{ID: o.ID, Label: o.Label, Consequence: o.Consequence})
	}
	if r.End == nil {
		return out
	}
	if r.End.Type == tickets.EventPauseAnswer {
		out.Answer = &wire.DetailAnswer{Source: r.End.Source, Choice: r.End.Choice, Text: r.End.Text, Note: r.End.Note}
	} else {
		out.ClosedReason = r.End.Reason
	}
	return out
}

// roundChat is the round as a conversation: the question, then the reply.
func roundChat(r wire.DetailRound) []wire.ChatMessage {
	msgs := []wire.ChatMessage{{From: r.By, Text: r.Summary}}
	if r.Answer != nil {
		reply := r.Answer.Choice
		if reply == "" {
			reply = r.Answer.Text
		}
		msgs = append(msgs, wire.ChatMessage{From: r.Answer.Source, Text: reply})
	}
	return msgs
}
