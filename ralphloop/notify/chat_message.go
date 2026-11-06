package notify

import (
	"github.com/elentok/gx/chatmarkup"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// urgency is the class a chat message belongs to; the batcher reads it.
type urgency string

const urgencyAttention urgency = "attention"

// chatMessage is one Chat message in the new layout: a plain Headline
// "<icon> <status> <subject>", detail lines, then the project alone.
type chatMessage struct {
	icon, status, subject string
	lines                 []string
	project               string
	kind                  string
	urgency               urgency
}

// Headline is the first line, before escaping.
func (m chatMessage) Headline() string {
	return m.icon + " " + m.status + " " + m.subject
}

// Text renders the message in style's dialect.
func (m chatMessage) Text(style chatmarkup.Style) chatmarkup.Text {
	return style.Layout(m.Headline(), m.lines, m.project)
}

// conflictStartedMessage is the "an agent took over a merge conflict" message.
func conflictStartedMessage(project, epic, ticketPath, identifier string) chatMessage {
	subject := epic + "/" + identifier
	if title := tickets.TicketTitle(ticketPath); title != "" {
		subject += " " + title
	}
	return chatMessage{
		icon: "🔀", status: "Conflict", subject: subject,
		lines:   []string{"An agent is resolving the merge conflict."},
		project: project,
		kind:    notifyKindConflictStarted,
	}
}

// ticketPauseMessage is the "a machine paused this ticket for a person"
// message: Answer for the answer class, Repair for any fault. status is the
// ticket's stored status word, old or new.
func ticketPauseMessage(project, epic, ticketPath, identifier, status, detail string) chatMessage {
	icon, word := "🙋", "Answer"
	if (schema.Ticket{Status: schema.Status(status)}).PausedClass() != schema.PauseClassAnswer {
		icon, word = "🛑", "Repair"
	}
	subject := epic + "/" + identifier
	if title := tickets.TicketTitle(ticketPath); title != "" {
		subject += " " + title
	}
	return chatMessage{
		icon: icon, status: word, subject: subject,
		lines:   []string{detail},
		project: project,
		kind:    notifyKindTicketNeedsHuman,
		urgency: urgencyAttention,
	}
}
