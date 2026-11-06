package notify

import (
	"strings"

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

// escalatedMessage is the "recovery gave up" message. failure is the failure's
// pause sentence for a ticket; an epic-level escalation (empty ticketPath)
// has no failure line of its own.
func escalatedMessage(project, epic, ticketPath, identifier, failure string) chatMessage {
	msg := chatMessage{
		icon: "⚠️", status: "Escalated", subject: epic,
		project: project,
		kind:    notifyKindTicketNeedsHuman,
		urgency: urgencyAttention,
	}
	if ticketPath == "" {
		msg.lines = []string{"All tickets are paused. Automatic recovery already ran once."}
		return msg
	}
	msg.subject += "/" + identifier
	if title := tickets.TicketTitle(ticketPath); title != "" {
		msg.subject += " " + title
	}
	msg.lines = []string{"Automatic recovery gave up. " + failure}
	return msg
}

// maxResultRunes bounds the Result a chat message carries.
const maxResultRunes = 1500

// resultMessage is a one-off's "done" message: its Result cut at
// maxResultRunes, or a plain line when none was recorded.
func resultMessage(project, epic, ticketPath, identifier, result string) chatMessage {
	subject := epic + "/" + identifier
	if title := tickets.TicketTitle(ticketPath); title != "" {
		subject += " " + title
	}
	result = strings.TrimSpace(result)
	if r := []rune(result); len(r) > maxResultRunes {
		result = string(r[:maxResultRunes]) + "…"
	}
	if result == "" {
		result = "No result recorded."
	}
	return chatMessage{
		icon: "✅", status: "Done", subject: subject,
		lines:   []string{result},
		project: project,
		kind:    notifyKindResult,
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
