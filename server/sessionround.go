package server

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/transcript"
)

const questionFence = "```gx-question"

// roundBlockNote explains a free-text card whose gx-question block was missing
// or bad.
const roundBlockNote = "The options could not be read."

// SessionRoundOption is one choice of an options card.
type SessionRoundOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Consequence string `json:"consequence,omitempty"`
}

// SessionRoundCard is one question of a Grilling round. Kind is yes-no,
// options or text; Note says why a card fell back to text.
type SessionRoundCard struct {
	Label       string               `json:"label"` // the header's question id without the Q: "8", or "8b"
	Title       string               `json:"title"`
	Body        string               `json:"body"`
	Kind        string               `json:"kind"`
	Yes         string               `json:"yes,omitempty"`
	No          string               `json:"no,omitempty"`
	Options     []SessionRoundOption `json:"options,omitempty"`
	Recommended string               `json:"recommended,omitempty"`
	Note        string               `json:"note,omitempty"`
}

// SessionNext is the phase a gx-design run-end line names and the Actionable
// that starts it.
type SessionNext struct {
	Phase  string            `json:"phase"`
	Action SessionActionable `json:"action"`
}

// SessionBackgroundTask is a shell, Monitor or subagent still running.
type SessionBackgroundTask struct {
	Kind      string `json:"kind"` // transcript.BackgroundTaskShell, ...Monitor or ...Agent
	Label     string `json:"label"`
	StartedAt string `json:"started_at"`
}

var (
	roundNumberRE = regexp.MustCompile(`Q(\d+)([a-z]?)`)
	boldSpanRE    = regexp.MustCompile(`\*\*(.+?)\*\*`)
	sentenceEndRE = regexp.MustCompile(`[.?!:](\s|$)`)
)

// splitFences removes every gx-question fence from text and returns the raw
// blocks it held, in order. An unclosed fence runs to the end of the text.
func splitFences(text string) (stripped string, blocks []string) {
	var kept []string
	inBlock := false
	for line := range strings.SplitSeq(text, "\n") {
		switch {
		case inBlock && strings.TrimSpace(line) == "```":
			inBlock = false
		case inBlock:
			blocks[len(blocks)-1] += line + "\n"
		case strings.TrimSpace(line) == questionFence:
			inBlock = true
			blocks = append(blocks, "")
		default:
			kept = append(kept, line)
		}
	}
	return strings.TrimSpace(strings.Join(kept, "\n")), blocks
}

// roundSegments splits an agent message holding a Grilling round into one
// segment per question. A segment runs from one marker to the next, or to a
// `---` line.
func roundSegments(text string) []string {
	var segs []string
	rest := text
	for {
		_, after, found := strings.Cut(rest, grillingMarker)
		if !found {
			return segs
		}
		end := len(after)
		if i := strings.Index(after, grillingMarker); i >= 0 {
			end = i
		}
		rest = after[end:]
		// The marker's trailing Q opens the header the number is read from.
		segs = append(segs, untilRule("Q"+after[:end]))
	}
}

// roundCards splits an agent message holding a Grilling round into cards.
func roundCards(text string) []SessionRoundCard {
	var cards []SessionRoundCard
	for i, seg := range roundSegments(text) {
		cards = append(cards, parseRoundSegment(i+1, seg).Card)
	}
	return cards
}

// untilRule cuts seg at its first `---` line.
func untilRule(seg string) string {
	var kept []string
	for line := range strings.SplitSeq(seg, "\n") {
		if strings.TrimSpace(line) == "---" {
			break
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// parsedRound is one parsed round segment. Err is set only when the segment
// holds gx-question blocks that cannot be read (Raw is their text); a segment
// with no block is plain text and has no Err.
type parsedRound struct {
	Card SessionRoundCard
	Err  error
	Raw  string
}

// parseRoundSegment parses one segment (the text after the marker). It is the
// single place that decides whether a block is readable, so the card shown and
// the bad block filed cannot disagree.
func parseRoundSegment(seq int, seg string) parsedRound {
	body, blocks := splitFences(seg)
	header, rest, _ := strings.Cut(body, "\n")
	card := SessionRoundCard{Label: strconv.Itoa(seq), Title: roundTitle(header), Body: strings.TrimSpace(roundHeaderText(header) + "\n" + rest), Kind: tickets.QuestionText}
	if m := roundNumberRE.FindStringSubmatch(header); m != nil {
		card.Label = m[1] + m[2]
	}
	res := parsedRound{Card: card}
	if len(blocks) == 0 {
		res.Card.Note = roundBlockNote
		return res
	}
	res.Raw = strings.TrimSpace(strings.Join(blocks, "\n"))
	if len(blocks) > 1 {
		res.Card.Note = roundBlockNote
		res.Err = fmt.Errorf("%d gx-question blocks, want one", len(blocks))
		return res
	}
	q, err := tickets.ParseQuestionBlock(blocks[0])
	if err != nil {
		res.Card.Note = roundBlockNote
		res.Err = err
		return res
	}
	c := &res.Card
	c.Kind, c.Yes, c.No, c.Recommended = q.Kind, q.Yes, q.No, q.Recommended
	for _, o := range q.Options {
		c.Options = append(c.Options, SessionRoundOption{ID: o.ID, Label: o.Label, Consequence: o.Consequence})
	}
	return res
}

// roundTitle reads a header leniently: the first bold span after the question
// number, else the first sentence.
func roundTitle(header string) string {
	rest := afterRoundNumber(header)
	if m := boldSpanRE.FindStringSubmatch(rest); m != nil {
		return strings.TrimSpace(m[1])
	}
	rest = strings.Trim(rest, "*-:– ")
	if loc := sentenceEndRE.FindStringIndex(rest); loc != nil {
		rest = rest[:loc[0]+1]
	}
	return strings.Trim(rest, "* :")
}

// roundHeaderText is what a header says after its number and leading bold
// title, which the card shows on their own: "Q3** - **Title**: Should I?"
// gives "Should I?". A header with no leading bold title keeps its text.
func roundHeaderText(header string) string {
	rest := afterRoundNumber(header)
	if loc := boldSpanRE.FindStringIndex(rest); loc != nil && loc[0] == 0 {
		rest = rest[loc[1]:]
	}
	return strings.TrimLeft(rest, "*-:– ")
}

// afterRoundNumber is a header after its question number, the bold that closes
// the number, and the separator before the title.
func afterRoundNumber(header string) string {
	rest := header
	if loc := roundNumberRE.FindStringIndex(header); loc != nil {
		rest = strings.TrimPrefix(header[loc[1]:], "**")
	}
	return strings.TrimLeft(rest, "-:– ")
}

// trailingAgentTexts lists the agent text messages after the person's last
// message, newest first.
func trailingAgentTexts(msgs []transcript.ChatMessage) []string {
	var out []string
	for i := len(msgs) - 1; i >= 0; i-- {
		switch m := msgs[i]; {
		case m.Role == transcript.ChatRolePerson || m.Kind == transcript.ChatKindCompacted:
			return out
		case m.Kind == transcript.ChatKindText:
			out = append(out, m.Text)
		}
	}
	return out
}

// chatRound is the round the agent's last turn ended with, or nil.
func chatRound(msgs []transcript.ChatMessage) []SessionRoundCard {
	for _, text := range trailingAgentTexts(msgs) {
		if strings.Contains(text, grillingMarker) {
			return roundCards(text)
		}
	}
	return nil
}

// chatNext is the run-end line of the agent's last turn, or nil.
func chatNext(msgs []transcript.ChatMessage, ticket string) *SessionNext {
	for _, text := range trailingAgentTexts(msgs) {
		if phase, a, ok := runEndNext(text, ticket); ok {
			return &SessionNext{Phase: phase, Action: a}
		}
	}
	return nil
}
