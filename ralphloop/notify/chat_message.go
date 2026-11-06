package notify

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/elentok/gx/chatmarkup"
	"github.com/elentok/gx/ralphloop/schedule"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// urgency is the class a chat message belongs to, by kind and never by icon;
// the batcher sorts by it, higher first. The zero value is information.
type urgency int

const (
	urgencyInformation urgency = iota // Progress, other notices, session Done, mute and test
	urgencyResult                     // Done, Merged, Complete, Ready
	urgencyConflict
	urgencyEscalated
	urgencyAnswer // Answer, Question, session Blocked, Rate-limited, Exited
	urgencyRepair // Repair and work-holding server notices
)

// chatMessage is one Chat message in the new layout: a plain Headline
// "<icon> <status> <subject>", detail lines, then the project alone.
type chatMessage struct {
	icon, status, subject string
	lines                 []string
	project               string
	kind                  string
	urgency               urgency
	// summaryWord overrides status as the word a batch summary lists, for a
	// message with no Status word of its own (a server notice).
	summaryWord string
	// repeat is how many identical copies a batch collapsed into this one;
	// above 1 the Headline ends in " ×N".
	repeat int
}

// Headline is the first line, before escaping.
func (m chatMessage) Headline() string {
	parts := []string{m.icon}
	for _, p := range []string{m.status, m.subject} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	headline := strings.Join(parts, " ")
	if m.repeat > 1 {
		headline += fmt.Sprintf(" ×%d", m.repeat)
	}
	return headline
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
	msg := chatMessage{icon: n.Emoji, subject: capitalize(n.Title), kind: n.Kind, summaryWord: "Notice"}
	if n.Detail != "" {
		msg.lines = []string{n.Detail}
	}
	if n.HoldsWork {
		msg.urgency = urgencyRepair
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
		lines:       lines,
		kind:        notifyKindTicketNeedsHuman,
		urgency:     urgencyRepair,
		summaryWord: "Repair",
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
		urgency: urgencyConflict,
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
		urgency: urgencyEscalated,
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

// mutedMessage is the edge-triggered per-source mute notice. source is the
// ticket's file path (store/project/epic/issues/<file>), which is the only
// place the server's sink can learn the epic and project from.
func mutedMessage(source, identifier string) chatMessage {
	issues := filepath.Dir(source)
	epicDir := filepath.Dir(issues)
	subject := filepath.Base(epicDir) + "/" + identifier
	if title := tickets.TicketTitle(source); title != "" {
		subject += " " + title
	}
	return chatMessage{
		icon: "🔇", status: "Muted", subject: subject,
		lines:   []string{"Too many messages. Muted until you re-enable it."},
		project: filepath.Base(filepath.Dir(epicDir)),
		kind:    notifyKindMuted,
	}
}

// globallyMutedMessage is the final notice sent on a transport before every
// further send on it is suppressed.
func globallyMutedMessage(transport string) chatMessage {
	return chatMessage{
		icon: "🚫", subject: "Muted all notifications",
		lines: []string{"Re-enable with gx notify --enable " + transport},
		kind:  notifyKindGloballyMuted,
	}
}

// testMessage is what `gx config test-notifications` sends.
func testMessage() chatMessage {
	return chatMessage{
		icon: "🔔", subject: "Test notification",
		lines: []string{"If you can see this, notifications are working."},
		kind:  "test",
	}
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
		urgency: urgencyResult,
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
		urgency: urgencyResult,
	}
}

// ProgressRunning is one running ticket in a Progress message.
type ProgressRunning struct {
	Identifier     string
	Title          string
	ElapsedSeconds int
}

// ProgressFacts is the plain data of one Progress update.
type ProgressFacts struct {
	Counts  schedule.EpicCounts
	Running []ProgressRunning
	// Finished* describe the tickets that finished since the last update;
	// FinishedElapsedSeconds is the sum of their elapsed times.
	FinishedCount          int
	FinishedElapsedSeconds int
	FinishedCost           float64
}

// progressMessage is the periodic "how is the epic going" message. Left is
// what is neither done nor running.
func progressMessage(project, epic string, f ProgressFacts) chatMessage {
	lines := make([]string, 0, 5)
	if len(f.Running) > 0 {
		parts := make([]string, len(f.Running))
		for i, r := range f.Running {
			parts[i] = fmt.Sprintf("%s %s (%s)", r.Identifier, r.Title, formatElapsed(r.ElapsedSeconds))
		}
		lines = append(lines, "Running: "+strings.Join(parts, ", "))
	}
	noun := "tickets"
	if f.FinishedCount == 1 {
		noun = "ticket"
	}
	lines = append(lines,
		fmt.Sprintf("Finished since the last update: %d %s · %s · %s", f.FinishedCount, noun, formatElapsed(f.FinishedElapsedSeconds), tickets.FormatCost(f.FinishedCost)),
		fmt.Sprintf("Left: %d", max(0, f.Counts.Total-f.Counts.Done-f.Counts.InProgress)),
		RenderCountsLine(f.Counts))
	return chatMessage{
		icon: "⏳", status: "Progress",
		subject: fmt.Sprintf("%s · %d of %d done", epic, f.Counts.Done, f.Counts.Total),
		lines:   lines,
		project: project,
		kind:    notifyKindProgress,
		urgency: urgencyInformation,
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
	class := urgencyRepair
	if word == "Answer" {
		class = urgencyAnswer
	}
	return chatMessage{
		icon: icon, status: word, subject: subject,
		lines:   []string{detail},
		project: project,
		kind:    notifyKindTicketNeedsHuman,
		urgency: class,
	}
}
