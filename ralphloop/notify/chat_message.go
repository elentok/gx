package notify

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/elentok/gx/chatmarkup"
	"github.com/elentok/gx/ralphloop/schedule"
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
	parts := []string{m.icon}
	for _, p := range []string{m.status, m.subject} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " ")
}

// capitalize upper-cases the first letter of s.
func capitalize(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// serverNoticeMessage is a server-level notice: an icon, the capitalized
// phrase and the detail, with no Status word and no project line.
func serverNoticeMessage(n ServerNotice) chatMessage {
	msg := chatMessage{icon: n.Emoji, subject: capitalize(n.Title), kind: n.Kind}
	if n.Detail != "" {
		msg.lines = []string{n.Detail}
	}
	if n.HoldsWork {
		msg.urgency = urgencyAttention
	}
	return msg
}

// pauseDigestMessage is the herdr outage digest: one line per held pause, no
// project line (each line names its own).
func pauseDigestMessage(lines []string) chatMessage {
	noun := "tickets"
	if len(lines) == 1 {
		noun = "ticket"
	}
	return chatMessage{
		icon: "🅿️", subject: fmt.Sprintf("%d %s paused while herdr was down", len(lines), noun),
		lines:   lines,
		kind:    notifyKindTicketNeedsHuman,
		urgency: urgencyAttention,
	}
}

// PauseDigestLine is one held pause in the digest: its project and ticket,
// the pause's Status word and its detail.
func PauseDigestLine(project, epic, identifier, status, detail string) string {
	_, word := pauseIconWord(status)
	return fmt.Sprintf("%s · %s/%s %s: %s", project, epic, identifier, word, detail)
}

// pauseIconWord is a pause's icon and Status word: Answer for the answer
// class, Repair for any fault. status is the ticket's stored status word.
func pauseIconWord(status string) (icon, word string) {
	if (schema.Ticket{Status: schema.Status(status)}).PausedClass() != schema.PauseClassAnswer {
		return "🛑", "Repair"
	}
	return "🙋", "Answer"
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

// formatElapsed renders seconds as "3h 20m", "1h 05m", "8m" or "42s".
func formatElapsed(seconds int) string {
	h, m := seconds/3600, seconds%3600/60
	switch {
	case h > 0:
		return fmt.Sprintf("%dh %02dm", h, m)
	case m > 0:
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%ds", seconds)
}

// epicOutcomeMessage is the "every ticket of the epic is done" message:
// Merged when its branch went onto its target, Complete otherwise, so chat
// never claims a merge that did not happen. The counts line stays last.
func epicOutcomeMessage(project, epic string, counts schedule.EpicCounts, elapsedSeconds int, totalCost float64, merged bool) chatMessage {
	icon, status, verb, ending := "✅", "Complete", "done", "Not merged."
	if merged {
		icon, status, verb, ending = "🎉", "Merged", "merged", ""
	}
	noun := "tickets"
	if counts.Done == 1 {
		noun = "ticket"
	}
	lines := []string{fmt.Sprintf("%d %s %s in %s · %s", counts.Done, noun, verb, formatElapsed(elapsedSeconds), tickets.FormatCost(totalCost))}
	if ending != "" {
		lines = append(lines, ending)
	}
	if counts.Recovered > 0 {
		lines = append(lines, fmt.Sprintf("%d recovered automatically", counts.Recovered))
	}
	lines = append(lines, RenderCountsLine(counts))
	return chatMessage{
		icon: icon, status: status, subject: epic,
		lines:   lines,
		project: project,
		kind:    notifyKindEpicComplete,
	}
}

// ticketPauseMessage is the "a machine paused this ticket for a person"
// message: Answer for the answer class, Repair for any fault. status is the
// ticket's stored status word, old or new.
func ticketPauseMessage(project, epic, ticketPath, identifier, status, detail string) chatMessage {
	icon, word := pauseIconWord(status)
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
