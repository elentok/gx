package tickets

import (
	"fmt"
	"strings"
)

// RenderRounds renders the "Park rounds" block of a ticket's log: earlier
// rounds one line each, the latest round in full. It returns "" when the log
// holds no park.question. Plain text, no ANSI: the CLI and the TUI preview
// share it.
func RenderRounds(events []LogEvent) string {
	rounds := Rounds(events)
	if len(rounds) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("Park rounds\n")
	last := len(rounds) - 1
	for i, r := range rounds[:last] {
		fmt.Fprintf(&b, "%d. %s: %s", i+1, roundState(r), r.Question.Summary)
		if outcome := roundOutcome(r); outcome != "" {
			fmt.Fprintf(&b, " -> %s", outcome)
		}
		b.WriteString("\n")
	}

	r := rounds[last]
	fmt.Fprintf(&b, "%d. %s: %s\n", last+1, roundState(r), r.Question.Summary)
	writeIndented(&b, r.Question.Text)
	if r.Question.Block != "" {
		writeIndented(&b, r.Question.Block)
	}
	if r.Question.Handoff != "" {
		b.WriteString("   Handoff:\n")
		writeIndented(&b, r.Question.Handoff)
	}
	if outcome := roundOutcome(r); outcome != "" {
		fmt.Fprintf(&b, "   %s\n", outcome)
	}
	return b.String()
}

func roundState(r ParkRound) string {
	switch {
	case r.End == nil:
		return "open"
	case r.End.Type == EventParkClosed:
		return "closed"
	default:
		return "answered"
	}
}

// roundOutcome is the answer or close reason of a finished round, "" while open.
func roundOutcome(r ParkRound) string {
	if r.End == nil {
		return ""
	}
	if r.End.Type == EventParkClosed {
		if r.End.Reason == "" {
			return "closed"
		}
		return "closed: " + r.End.Reason
	}
	parts := make([]string, 0, 2)
	for _, p := range []string{r.End.Choice, r.End.Note} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return "answer: (empty)"
	}
	return "answer: " + strings.Join(parts, " - ")
}

func writeIndented(b *strings.Builder, text string) {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return
	}
	for line := range strings.SplitSeq(text, "\n") {
		if line == "" {
			b.WriteString("\n")
			continue
		}
		b.WriteString("   ")
		b.WriteString(line)
		b.WriteString("\n")
	}
}
