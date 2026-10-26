package grillround

import (
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/transcript"
)

// Item is one counted Grilling round or closing summary. A summary has no
// questions.
type Item struct {
	MessageID string
	At        time.Time
	Questions int
	Words     int
}

var (
	linkRE       = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	listNumberRE = regexp.MustCompile(`(?m)^\s*\d+[.)]\s`)
	questionNoRE = regexp.MustCompile(`^Q\d+[a-z]?`)
	glyphs       = strings.NewReplacer("❓", " ", "➡️", " ", "➡", " ", "*", "", "`", "")
)

// Count turns a conversation's chat messages into counted items: one per
// Grilling round, and one per closing summary. msgs is the full ReadChat
// output, before any cap or fence stripping.
func Count(msgs []transcript.ChatMessage) []Item {
	var items []Item
	answered := false // the turn before this one ended with an answered round that asks for a summary
	for _, turn := range turns(msgs) {
		var turnItems []Item
		turnItems, answered = countTurn(turn, answered)
		items = append(items, turnItems...)
	}
	return items
}

// turns groups the agent text messages between two person messages. The first
// turn is the agent text before any person message.
func turns(msgs []transcript.ChatMessage) [][]transcript.ChatMessage {
	turns := [][]transcript.ChatMessage{nil}
	for _, m := range msgs {
		switch {
		case m.Role == transcript.ChatRolePerson:
			turns = append(turns, nil)
		case m.Role == transcript.ChatRoleAgent && m.Kind == transcript.ChatKindText:
			turns[len(turns)-1] = append(turns[len(turns)-1], m)
		}
	}
	return turns
}

// countTurn counts one agent turn. wantSummary says the person just answered a
// round: the last text before the run-end line or the next round is the summary.
// asksSummary is true when the turn's last round is a straightforward one, so
// the turn after the person's answer closes with a summary.
func countTurn(turn []transcript.ChatMessage, wantSummary bool) (items []Item, asksSummary bool) {
	var candidate *Item
	terminated := false
	for _, m := range turn {
		runEnd := RunEndLine.FindStringIndex(m.Text)
		hasRound := HasMarker(m.Text)
		if !hasRound && runEnd == nil {
			if !terminated {
				candidate = summaryItem(m, m.Text)
			}
			continue
		}
		cut := len(m.Text)
		if runEnd != nil {
			cut = runEnd[0]
		}
		if hasRound {
			cut = min(cut, markerRE.FindStringIndex(m.Text)[0])
		}
		shared := hasRound && isShared(m.Text)
		if !terminated && wantSummary && !shared {
			if c := summaryItem(m, m.Text[:cut]); c != nil {
				candidate = c
			}
			if candidate != nil {
				items = append(items, *candidate)
			}
		}
		terminated = true
		if hasRound {
			items = append(items, roundItem(m, shared))
			asksSummary = !shared
		}
	}
	return items, asksSummary
}

func summaryItem(m transcript.ChatMessage, text string) *Item {
	words := countWords(text)
	if words == 0 {
		return nil
	}
	return &Item{MessageID: m.ID, At: m.At, Words: words}
}

// isShared reports whether a round is the "Shared understanding" question that
// closes a grilling that was not straightforward.
func isShared(text string) bool {
	for _, seg := range Segments(text) {
		header, _, _ := strings.Cut(seg, "\n")
		if strings.Contains(strings.ToLower(header), "shared understanding") {
			return true
		}
	}
	return false
}

// roundItem counts the questions and words of a message holding a round. The
// text before the first card counts only for a Shared understanding round.
func roundItem(m transcript.ChatMessage, shared bool) Item {
	segs := Segments(m.Text)
	item := Item{MessageID: m.ID, At: m.At, Questions: len(segs)}
	if shared {
		item.Words += countWords(m.Text[:markerRE.FindStringIndex(m.Text)[0]])
	}
	for _, seg := range segs {
		item.Words += segmentWords(seg)
	}
	item.Words += countWords(defaultsList(m.Text))
	return item
}

// segmentWords counts what a card shows: its text without the question number,
// and the values of its gx-question block.
func segmentWords(seg string) int {
	body, blocks := SplitFences(seg)
	words := countWords(questionNoRE.ReplaceAllString(body, ""))
	for _, block := range blocks {
		q, err := tickets.ParseQuestionBlock(block)
		if err != nil {
			continue
		}
		words += countWords(q.Yes) + countWords(q.No)
		for _, o := range q.Options {
			words += countWords(o.Label) + countWords(o.Consequence)
		}
	}
	return words
}

// countWords counts the whitespace-separated pieces of text that hold a letter
// or a digit, after stripping markdown markers and the card glyphs.
func countWords(text string) int {
	text = linkRE.ReplaceAllString(text, "$1")
	text = listNumberRE.ReplaceAllString(text, "")
	text = glyphs.Replace(text)
	n := 0
	for piece := range strings.FieldsSeq(text) {
		if strings.ContainsFunc(piece, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) {
			n++
		}
	}
	return n
}
