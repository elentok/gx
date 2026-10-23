package server

import (
	"net/http"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// Actions the detail route offers. The server decides them; the UI renders them.
const (
	ActionAnswer  = "answer"
	ActionApprove = "approve"
	ActionResume  = "resume"
)

// Outcomes of a pause round.
const (
	RoundOpen     = "open"
	RoundAnswered = "answered"
	RoundClosed   = "closed"
)

// DetailOption is one choice of an options question.
type DetailOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Consequence string `json:"consequence,omitempty"`
}

// DetailRound is one pause round. The earlier rounds fill only Seq, By, Summary
// and Outcome; the latest fills the rest.
type DetailRound struct {
	Seq     int    `json:"seq"`
	By      string `json:"by"`
	Summary string `json:"summary"`
	Outcome string `json:"outcome"`

	Kind        string         `json:"kind,omitempty"`
	Text        string         `json:"text,omitempty"`
	Block       string         `json:"block,omitempty"`
	Handoff     string         `json:"handoff,omitempty"`
	Options     []DetailOption `json:"options,omitempty"`
	Recommended string         `json:"recommended,omitempty"`
	Yes         string         `json:"yes,omitempty"`
	No          string         `json:"no,omitempty"`

	// Answer is the pause.answer that ended the round.
	Answer *DetailAnswer `json:"answer,omitempty"`
	// ClosedReason is the pause.closed reason of a round no answer ended.
	ClosedReason string `json:"closed_reason,omitempty"`
}

// DetailAnswer is a person's reply to a round.
type DetailAnswer struct {
	Source string `json:"source,omitempty"`
	Choice string `json:"choice,omitempty"`
	Text   string `json:"text,omitempty"`
	Note   string `json:"note,omitempty"`
}

// PermissionCard is the pending permission prompt of a blocked agent.
type PermissionCard struct {
	RequestID   string `json:"request_id"`
	Tool        string `json:"tool,omitempty"`
	Input       string `json:"input,omitempty"`
	Description string `json:"description,omitempty"`
	// Hint is set when the prompt cannot be approved from here; the card carries
	// no approve action then.
	Hint string `json:"hint,omitempty"`
}

const permissionHint = "Approve it in the TUI"

// ChatMessage is one message of the latest round's exchange.
type ChatMessage struct {
	From string `json:"from"` // "agent", "gx", or the answer's source
	Text string `json:"text"`
}

// TicketDetail is the /v1/tickets/detail payload.
type TicketDetail struct {
	Address string `json:"address"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	// PauseReason picks the action the page labels: Answer, Choose base, Retry or Resume.
	PauseReason string `json:"pause_reason,omitempty"`
	// PauseLabel is PauseReason in the plain words the summaries and Queue show.
	PauseLabel string `json:"pause_label,omitempty"`
	// PauseText is the body's "## Paused" section; the body keeps it too, so the
	// client leaves it out of the description.
	PauseText string `json:"pause_text,omitempty"`
	Type      string `json:"type,omitempty"`
	Epic      string `json:"epic"`
	// MapEpic is set for a ticket of a map epic, which has no stats and never pauses.
	MapEpic bool `json:"map_epic"`
	// Body is the whole body markdown, comments included.
	Body    string          `json:"body,omitempty"`
	Related []RelatedTicket `json:"related"`
	Stats   *DetailStats    `json:"stats,omitempty"`
	// Answered is derived, never stored: the ticket is open again and its
	// latest question has an answer.
	Answered bool `json:"answered"`
	// Rounds holds every round before the latest, oldest first.
	Rounds []DetailRound `json:"rounds"`
	Latest *DetailRound  `json:"latest,omitempty"`
	// Permission is empty until a headless or pty agent reports a pending prompt.
	Permission *PermissionCard `json:"permission,omitempty"`
	Chat       []ChatMessage   `json:"chat"`
	// Runs is one line per run, oldest first; a map ticket has none.
	Runs []DetailRun `json:"runs"`
	// State is the token an answer or resume sends back; empty without a round.
	State string `json:"state,omitempty"`
	// Bases are the competing blockers an ambiguous-base pause chooses between;
	// the resume sends the chosen one back as its base.
	Bases   []string `json:"bases,omitempty"`
	Actions []string `json:"actions"`
	// ErrorReason, ParseError and Path are set for an Error ticket. ParseError is
	// set only when the file fails to parse, and then the payload carries no body
	// or stats.
	ErrorReason string `json:"error_reason,omitempty"`
	ParseError  string `json:"parse_error,omitempty"`
	Path        string `json:"path,omitempty"`
}

func (s *Server) detail(w http.ResponseWriter, r *http.Request) {
	address := r.URL.Query().Get("address")
	// A malformed address is a ticket nobody has, the same as an unknown one.
	if _, err := tickets.ParseAddress(address, tickets.AddressContext{}); err != nil {
		http.Error(w, "no ticket "+address, http.StatusNotFound)
		return
	}
	ref, ok, err := s.findTicket(address)
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

func (s *Server) ticketDetail(ref ticketRef) (TicketDetail, error) {
	t, err := schema.ParseTicket(ref.ticket.Path)
	if err != nil {
		return brokenDetail(ref, err), nil
	}
	evs, err := tickets.ReadLog(ref.ticket.Path)
	if err != nil {
		return TicketDetail{}, err
	}
	d := TicketDetail{
		Address: ref.addr.String(), Title: ref.ticket.Title, Status: string(storedStatus(t)), PauseReason: string(t.PauseReason),
		Rounds: []DetailRound{}, Chat: []ChatMessage{}, Actions: []string{}, Runs: []DetailRun{},
		PauseLabel: t.PauseReason.Label(), Type: ref.ticket.Type, Epic: rootOf(ref.addr).String(),
		MapEpic: ref.epic.IsMap, Body: ref.ticket.Body,
	}
	if ref.epic.RenderedStatus(ref.ticket) == tickets.StatusError {
		d.Status = tickets.StatusError.Word()
		d.ErrorReason = tickets.PauseDetail(ref.epic, ref.ticket)
		d.Path = ref.ticket.Path
	}
	if t.PausedClass() != "" {
		d.PauseText = schema.Section(ref.ticket.Body, "Paused")
	}
	if !ref.epic.IsMap {
		d.Stats = s.detailStats(ref)
		if d.Runs, err = s.ticketRuns(ref.addr, t); err != nil {
			return TicketDetail{}, err
		}
	}
	epics, err := s.projectEpics(ref.addr.Project)
	if err != nil {
		return TicketDetail{}, err
	}
	d.Related = relatedTickets(ref, epics)
	if t.Status == schema.StatusClaimed {
		if d.Permission, err = s.permissionCard(ref.addr); err != nil {
			return TicketDetail{}, err
		}
	}
	rounds := tickets.Rounds(evs)
	if len(rounds) == 0 {
		d.Actions = detailActions(t, nil, false, d.Permission)
		return d, nil
	}
	last := rounds[len(rounds)-1]
	for _, r := range rounds[:len(rounds)-1] {
		d.Rounds = append(d.Rounds, detailRound(r, false))
	}
	latest := detailRound(last, true)
	d.Latest = &latest
	d.State = answerState(last.Question, t.PausedClass())
	d.Answered = t.Status == schema.StatusOpen && latest.Outcome == RoundAnswered
	d.Chat = roundChat(latest)
	gone := false
	if t.PausedClass() != "" {
		if gone, err = s.agentGone(d.Address); err != nil {
			return TicketDetail{}, err
		}
	}
	d.Actions = detailActions(t, &last, gone, d.Permission)
	if t.PausedClass() == schema.PauseClassAnswer && t.PauseReason == schema.PauseAmbiguousBase {
		d.Bases = s.ambiguousBlockers(ref.addr)
	}
	return d, nil
}

// brokenDetail is the small payload of a file that fails to parse: nothing the
// body or the log could add.
func brokenDetail(ref ticketRef, parseErr error) TicketDetail {
	return TicketDetail{
		Address: ref.addr.String(), Title: ref.ticket.Title, Status: tickets.StatusError.Word(),
		Epic: rootOf(ref.addr).String(), MapEpic: ref.epic.IsMap,
		ErrorReason: tickets.PauseDetail(ref.epic, ref.ticket), ParseError: parseErr.Error(), Path: ref.ticket.Path,
		Related: []RelatedTicket{}, Rounds: []DetailRound{}, Chat: []ChatMessage{}, Actions: []string{}, Runs: []DetailRun{},
	}
}

// permissionCard is the pending prompt of addr's live agent, or nil. It reads
// the runner's status, so it runs only when a detail or a snapshot is built.
// A prompt the web cannot approve (a herdr-hosted agent, an unnamed dialog, a
// tool that needs a real answer) gets a card with a hint and no request ID.
func (s *Server) permissionCard(addr tickets.Address) (*PermissionCard, error) {
	runner, sess, ok, err := s.liveSession(addr)
	if err != nil || !ok {
		return nil, err
	}
	st, err := runner.Status(sess)
	if err != nil || st.State != agentrunner.StateBlocked {
		return nil, nil
	}
	p := st.Pending
	if p == nil {
		return &PermissionCard{Description: st.BlockedReason, Hint: permissionHint}, nil
	}
	card := &PermissionCard{RequestID: p.ID, Tool: p.Tool, Input: string(p.Input), Description: p.Description}
	if p.NotApprovable || runner.Name() == string(agentrunner.ChoiceHerdr) {
		card.RequestID, card.Hint = "", permissionHint
	}
	return card, nil
}

// detailActions is what a person may do with the ticket right now. Approve
// joins when the permission card can be approved from here.
func detailActions(t schema.Ticket, latest *tickets.PauseRound, agentGone bool, card *PermissionCard) []string {
	actions := []string{}
	if card != nil && card.RequestID != "" {
		actions = append(actions, ActionApprove)
	}
	if t.PausedClass() == "" {
		return actions
	}
	selfReported := t.PauseReason == schema.PauseSelfReported
	if selfReported && t.PausedClass() == schema.PauseClassAnswer && latest != nil && latest.End == nil {
		actions = append(actions, ActionAnswer)
	}
	if !selfReported || agentGone {
		actions = append(actions, ActionResume)
	}
	return actions
}

// detailRound reads a round's question leniently: a bad block is free text.
func detailRound(r tickets.PauseRound, full bool) DetailRound {
	out := DetailRound{Seq: r.Question.Seq, By: r.Question.By, Summary: r.Question.Summary, Outcome: RoundOpen}
	if r.End != nil {
		out.Outcome = RoundClosed
		if r.End.Type == tickets.EventPauseAnswer {
			out.Outcome = RoundAnswered
		}
	}
	if !full {
		return out
	}
	q := tickets.QuestionOfEvent(r.Question)
	out.Kind, out.Text, out.Block, out.Handoff = q.Kind, q.Detail, r.Question.Block, q.Handoff
	out.Recommended, out.Yes, out.No = q.Recommended, q.Yes, q.No
	for _, o := range q.Options {
		out.Options = append(out.Options, DetailOption{ID: o.ID, Label: o.Label, Consequence: o.Consequence})
	}
	if r.End == nil {
		return out
	}
	if r.End.Type == tickets.EventPauseAnswer {
		out.Answer = &DetailAnswer{Source: r.End.Source, Choice: r.End.Choice, Text: r.End.Text, Note: r.End.Note}
	} else {
		out.ClosedReason = r.End.Reason
	}
	return out
}

// roundChat is the round as a conversation: the question, then the reply.
func roundChat(r DetailRound) []ChatMessage {
	msgs := []ChatMessage{{From: r.By, Text: r.Summary}}
	if r.Answer != nil {
		reply := r.Answer.Choice
		if reply == "" {
			reply = r.Answer.Text
		}
		msgs = append(msgs, ChatMessage{From: r.Answer.Source, Text: reply})
	}
	return msgs
}
