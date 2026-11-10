package server

import (
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/tickets/ticketlog"
)

// chatSource is the chat's view of the server's sessions.
type chatSource struct{ s *Server }

func (c chatSource) SessionDir(id string) (string, chat.Meta, wire.SessionResult, error) {
	dir, m, res, err := c.s.sessions.SessionDir(id)
	return dir, m.ChatMeta(), res, err
}

func (c chatSource) ReadableSession(id string) (string, chat.Meta, wire.SessionResult, error) {
	dir, m, res, err := c.s.sessions.ReadableSession(id)
	return dir, m.ChatMeta(), res, err
}

func (c chatSource) ReadMeta(dir string) (chat.Meta, error) {
	m, err := sessions.ReadMeta(dir)
	return m.ChatMeta(), err
}

func (c chatSource) CheckProjectDir(project, dir string) (*wire.SessionResult, error) {
	return c.s.sessions.CheckProjectDir(project, dir)
}

// TicketAsk applies the same answerable checks as ticketops.Ops.Answer, so a
// card is listed exactly while an answer would be accepted.
func (c chatSource) TicketAsk(address string) (ticketlog.Question, string, bool) {
	ref, found, err := c.s.writer.FindTicket(address)
	if err != nil || !found {
		return ticketlog.Question{}, "", false
	}
	t, err := schema.ParseTicket(ref.Ticket.Path)
	if err != nil || t.PausedClass() == "" || t.PausedClass() == schema.PauseClassRepair || t.PauseReason != schema.PauseSelfReported {
		return ticketlog.Question{}, "", false
	}
	open := tickets.OpenQuestion(queue.ReadLogLenient(ref.Ticket))
	if open == nil {
		return ticketlog.Question{}, "", false
	}
	return ticketlog.QuestionOfEvent(*open), queue.AnswerState(*open, t.PausedClass()), true
}

func (c chatSource) HeldAsks(session string) []wire.AskHookRequest {
	return c.s.sessions.HeldAsks(session)
}
func (c chatSource) AskReleased(session, question string) bool {
	return c.s.sessions.AskReleased(session, question)
}
func (c chatSource) StepFinal(ref chat.FlowRef) bool { return c.s.prompt.StepFinal(ref) }
func (c chatSource) NoteFailure(id string)           { c.s.sessions.NoteFailure(id) }
func (c chatSource) NoteCards(id string)             { c.s.sessions.NoteQuestionCards(id) }
func (c chatSource) Stopped() <-chan struct{}        { return c.s.sessions.Watches.Stop }
func (c chatSource) CodexHome() (string, error)      { return c.s.projects.CodexHome() }
func (c chatSource) Publish(event, id string)        { c.s.events.Publish(event, id) }
func (c chatSource) HasChatReader(h string) bool     { return c.s.settings.Load().HasChatReader(h) }
