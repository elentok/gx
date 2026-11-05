package chat

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/grillround"
	"github.com/elentok/gx/tickets/ticketlog"
	"github.com/elentok/gx/transcript"
)

// roundBlockNote explains a free-text card whose gx-question block was missing
// or bad.
const roundBlockNote = "The options could not be read."

var (
	roundNumberRE = regexp.MustCompile(`Q(\d+)([a-z]?)`)
	boldSpanRE    = regexp.MustCompile(`\*\*(.+?)\*\*`)
	sentenceEndRE = regexp.MustCompile(`[.?!:](\s|$)`)
)

// RoundCards splits an agent message holding a Grilling round into cards.
func RoundCards(text string) []wire.SessionRoundCard {
	var cards []wire.SessionRoundCard
	for i, seg := range grillround.Segments(text) {
		cards = append(cards, ParseRoundSegment(i+1, seg).Card)
	}
	return cards
}

// ParsedRound is one parsed round segment. Err is set only when the segment
// holds gx-question blocks that cannot be read (Raw is their text); a segment
// with no block is plain text and has no Err.
type ParsedRound struct {
	Card wire.SessionRoundCard
	Err  error
	Raw  string
}

// ParseRoundSegment parses one segment (the text after the marker). It is the
// single place that decides whether a block is readable, so the card shown and
// the bad block filed cannot disagree.
func ParseRoundSegment(seq int, seg string) ParsedRound {
	body, blocks := grillround.SplitFences(seg)
	header, rest, _ := strings.Cut(body, "\n")
	card := wire.SessionRoundCard{Label: strconv.Itoa(seq), Title: roundTitle(header), Body: strings.TrimSpace(roundHeaderText(header) + "\n" + rest), Kind: ticketlog.QuestionText}
	if m := roundNumberRE.FindStringSubmatch(header); m != nil {
		card.Label = m[1] + m[2]
	}
	res := ParsedRound{Card: card}
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
	q, err := ticketlog.ParseQuestionBlock(blocks[0])
	if err != nil {
		res.Card.Note = roundBlockNote
		res.Err = err
		return res
	}
	c := &res.Card
	c.Kind, c.Yes, c.No, c.Recommended = q.Kind, q.Yes, q.No, q.Recommended
	for _, o := range q.Options {
		c.Options = append(c.Options, wire.SessionRoundOption{ID: o.ID, Label: o.Label, Consequence: o.Consequence})
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

// chatRound is the round the agent's last turn ended with, or nil.
func chatRound(msgs []transcript.ChatMessage) []wire.SessionRoundCard {
	for _, text := range transcript.TrailingAgentTexts(msgs) {
		if grillround.HasMarker(text) {
			return RoundCards(text)
		}
	}
	return nil
}

// ChatNext is the run-end line of the agent's last turn, or nil.
func ChatNext(msgs []transcript.ChatMessage, kind agentrunner.Kind, ticket string, final bool) *wire.SessionNext {
	for _, text := range transcript.TrailingAgentTexts(msgs) {
		if a, ok := RunEndNext(text, ticket, string(kind), final); ok {
			return &wire.SessionNext{Action: a}
		}
	}
	return nil
}
