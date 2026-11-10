package chat

import (
	"strings"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets/ticketlog"
)

// CardSlot is one place a Question card can come from, in the order the
// builder shows them: a live blocker first, then a ticket ask, then a printed
// round.
type CardSlot int

const (
	SlotLiveBlocker CardSlot = iota
	SlotTicketAsk
	SlotRound
)

// SessionClass decides which card slots apply to a session.
type SessionClass int

const (
	ClassPlain SessionClass = iota
	ClassFlow               // Design or Triage
	ClassQueueAgent
	ClassFixAgent
)

// classSlots is the class table: later tickets add a slot to a class here.
var classSlots = map[SessionClass][]CardSlot{
	ClassPlain:      {SlotLiveBlocker, SlotRound},
	ClassFlow:       {SlotLiveBlocker, SlotRound},
	ClassQueueAgent: {SlotTicketAsk},
	ClassFixAgent:   {SlotLiveBlocker, SlotTicketAsk, SlotRound},
}

// askAddress is the ticket whose ask a session's chat lists: the run of a Queue
// agent, or what a Fix agent fixes (which may be a project name, then no ask).
func askAddress(m Meta) string {
	if m.RunTicket != "" {
		return m.RunTicket
	}
	return m.FixOf
}

// TicketAskCard is an open ticket ask as a Question card. Its id is the ticket
// address and the ask's state token, so a new or re-paused ask is a new card.
func TicketAskCard(address string, q ticketlog.Question, state string) []wire.QuestionCard {
	cq := wire.QuestionCardQuestion{
		Kind: q.Kind, Title: q.Summary, Body: q.Detail, Yes: q.Yes, No: q.No,
		Recommended: q.Recommended,
	}
	for _, l := range q.Links {
		cq.Links = append(cq.Links, wire.QuestionLink{Label: l.Label, URL: l.URL})
	}
	for _, o := range q.Options {
		cq.Options = append(cq.Options, wire.QuestionCardOption{ID: o.ID, Label: o.Label, Consequence: o.Consequence})
	}
	return []wire.QuestionCard{{Source: wire.CardSourceTicketAsk, ID: address + "#" + state, Questions: []wire.QuestionCardQuestion{cq}}}
}

// SplitTicketAskID is the ticket address and state token of a ticket-ask card id.
func SplitTicketAskID(id string) (address, state string, ok bool) {
	return strings.Cut(id, "#")
}

// ClassOf is the class of a session.
func ClassOf(m Meta) SessionClass {
	switch {
	case strings.HasPrefix(m.ID, queue.AgentIDPrefix):
		return ClassQueueAgent
	case m.FixOf != "":
		return ClassFixAgent
	case m.Flow.Flow != "":
		return ClassFlow
	}
	return ClassPlain
}

// CardSources holds the cards each slot produced for one session.
type CardSources map[CardSlot][]wire.QuestionCard

// BuildCards picks the cards a session of the class shows: the slots that
// apply, in slot order, and only the live blocker while one is open. It is
// never nil.
func BuildCards(class SessionClass, src CardSources) []wire.QuestionCard {
	return PickCards(classSlots[class], src)
}

// PickCards is BuildCards for a given list of applying slots.
func PickCards(slots []CardSlot, src CardSources) []wire.QuestionCard {
	out := []wire.QuestionCard{}
	for _, slot := range slots {
		cards := src[slot]
		if slot == SlotLiveBlocker && len(cards) > 0 {
			return append(out, cards...)
		}
		out = append(out, cards...)
	}
	return out
}

// RoundCard is the printed round as one Question card, or no cards. Its id is
// the round's labels and titles, so it holds across reads and changes with a
// new round.
func RoundCard(round []wire.SessionRoundCard) []wire.QuestionCard {
	if len(round) == 0 {
		return nil
	}
	keys := make([]string, len(round))
	qs := make([]wire.QuestionCardQuestion, len(round))
	for i, c := range round {
		keys[i] = c.Label + ":" + c.Title
		q := wire.QuestionCardQuestion{
			Kind: c.Kind, Label: c.Label, Title: c.Title, Body: c.Body, Yes: c.Yes, No: c.No,
			Recommended: c.Recommended, Links: c.Links, Note: c.Note,
		}
		for _, o := range c.Options {
			q.Options = append(q.Options, wire.QuestionCardOption{ID: o.ID, Label: o.Label, Consequence: o.Consequence})
		}
		qs[i] = q
	}
	return []wire.QuestionCard{{Source: wire.CardSourceRound, ID: strings.Join(keys, "|"), Questions: qs}}
}

// SessionCards is the questions list of a session's chat response.
func SessionCards(m Meta, round []wire.SessionRoundCard) []wire.QuestionCard {
	return BuildCards(ClassOf(m), CardSources{SlotRound: RoundCard(round)})
}
