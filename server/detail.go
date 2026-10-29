package server

import (
	"net/http"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// Actions the detail route offers. The server decides them; the UI renders them.
const (
	ActionAnswer         = "answer"
	ActionApprove        = "approve"
	ActionResume         = "resume"
	ActionRetry          = "retry"
	ActionStartOver      = "start-over"
	ActionMerge          = "merge"
	ActionChooseBase     = "choose-base"
	ActionReopenAndRetry = "reopen-and-retry"
	ActionStashMerge     = "stash-merge"
)

// Outcomes of a pause round.
const (
	RoundOpen     = "open"
	RoundAnswered = "answered"
	RoundClosed   = "closed"
)

// ActionInfo is the text of one action's button.
type ActionInfo struct {
	Label   string `json:"label"`
	Tooltip string `json:"tooltip"`
}

// actionInfo looks up each action's label and tooltip.
func actionInfo(actions []string) map[string]ActionInfo {
	out := make(map[string]ActionInfo, len(actions))
	for _, a := range actions {
		if v, ok := schema.VerbOfWord(a); ok {
			out[a] = ActionInfo{Label: v.Label(), Tooltip: v.Tooltip()}
		}
	}
	return out
}

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

	// PromptedAt is when a person last prompted the running agent of this open
	// round, and Prompt is the line the web shows for it.
	PromptedAt *time.Time `json:"prompted_at,omitempty"`
	Prompt     string     `json:"prompt,omitempty"`
}

// DetailAnswer is a person's reply to a round.
type DetailAnswer struct {
	Source string `json:"source,omitempty"`
	Choice string `json:"choice,omitempty"`
	Text   string `json:"text,omitempty"`
	Note   string `json:"note,omitempty"`
}

// DetailRecommendation is what an investigate agent left on a paused ticket: its
// diagnosis and the one verb of Actions it recommends.
type DetailRecommendation struct {
	Verb      string `json:"verb"`
	Diagnosis string `json:"diagnosis"`
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
	// HasScreen is set when the latest run's agent ran in a pty, so
	// /v1/sessions/screen?ticket= has a screen to show.
	HasScreen bool `json:"has_screen,omitempty"`
	// State is the token an answer or resume sends back; empty without a round.
	State string `json:"state,omitempty"`
	// Bases are the competing blockers an ambiguous-base pause chooses between;
	// the resume sends the chosen one back as its base.
	Bases   []string `json:"bases,omitempty"`
	Actions []string `json:"actions"`
	// Recommendation is set while the open pause round carries one.
	Recommendation *DetailRecommendation `json:"recommendation,omitempty"`
	// ActionInfo gives each entry of Actions its button label and tooltip, from
	// the pause-to-verb table, so the page holds no verb text of its own.
	ActionInfo map[string]ActionInfo `json:"action_info,omitempty"`
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
	d := newTicketDetail(ref)
	fillHeader(&d, ref, t)
	if !ref.epic.IsMap {
		d.Stats = s.detailStats(ref)
		_, d.HasScreen = s.ticketPtyDir(ref.addr)
		var writtenAt time.Time
		if t.Status == schema.StatusCancelled {
			writtenAt = ticketWrittenAt(ref.ticket.Path)
		}
		if d.Runs, err = s.ticketRuns(ref, t, evs, writtenAt); err != nil {
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
	d.State = detailState(evs, t)
	if rec := tickets.RecommendationOf(evs); rec != nil && t.PausedClass() != "" {
		d.Recommendation = &DetailRecommendation{Verb: rec.Choice, Diagnosis: rec.Text}
	}
	rounds := tickets.Rounds(evs)
	if d.Actions, err = s.detailFor(ref.addr.String(), t, rounds, d.Permission); err != nil {
		return TicketDetail{}, err
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
	d.Answered = t.Status == schema.StatusOpen && latest.Outcome == RoundAnswered
	d.Chat = roundChat(latest)
	if t.PausedClass() == schema.PauseClassAnswer && t.PauseReason == schema.PauseAmbiguousBase {
		d.Bases = s.ambiguousBlockers(ref.addr)
	}
	return d, nil
}

// detailFor is what a person may do with a parsed ticket now, given its pause
// rounds. The ticket detail and ticketRecommend both read their actions from here.
func (s *Server) detailFor(addr string, t schema.Ticket, rounds []tickets.PauseRound, card *PermissionCard) ([]string, error) {
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
func brokenDetail(ref ticketRef, parseErr error) TicketDetail {
	d := newTicketDetail(ref)
	d.Status = tickets.StatusError.Word()
	d.ErrorReason = "ticket file format is invalid"
	d.ParseError = parseErr.Error()
	d.Path = ref.ticket.Path
	return d
}

// newTicketDetail is the payload every ticket starts from: what the address
// alone gives, with every list empty rather than null.
func newTicketDetail(ref ticketRef) TicketDetail {
	return TicketDetail{
		Address: ref.addr.String(), Title: ref.ticket.Title, Epic: rootOf(ref.addr).String(), MapEpic: ref.epic.IsMap,
		Related: []RelatedTicket{}, Rounds: []DetailRound{}, Chat: []ChatMessage{}, Actions: []string{}, Runs: []DetailRun{},
	}
}

// fillHeader sets the fields of a ticket that parsed: its status, pause, body
// and, for an Error ticket, the reason.
func fillHeader(d *TicketDetail, ref ticketRef, t schema.Ticket) {
	d.Status = string(storedStatus(t))
	d.PauseReason = string(t.PauseReason)
	d.PauseLabel = t.PauseReason.Label()
	d.Type = ref.ticket.Type
	d.Body = ref.ticket.Body
	if ref.epic.RenderedStatus(ref.ticket) == tickets.StatusError {
		d.Status = tickets.StatusError.Word()
		d.ErrorReason = tickets.PauseDetail(ref.epic, ref.ticket)
		d.Path = ref.ticket.Path
	}
	if t.PausedClass() != "" {
		d.PauseText = schema.Section(ref.ticket.Body, "Paused")
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
	if t.FinishedCommitless() {
		actions = append(actions, ActionMerge)
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
