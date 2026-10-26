package server

import (
	"strings"
	"testing"

	"github.com/elentok/gx/grillround"
)

func TestParseRoundSegment_BadBlockAndCleanCardAreExclusive(t *testing.T) {
	const fence = "```gx-question\n"
	cases := []struct {
		name     string
		seg      string
		wantBad  bool
		wantNote bool
	}{
		{"readable", "Q1 - **Ok?**: x\n\n" + fence + "type: yes-no\n```", false, false},
		{"no block", "Q1 - **Plain?**: x", false, true},
		{"many blocks", "Q1 - **Two?**: x\n\n" + fence + "type: yes-no\n```\n\n" + fence + "type: yes-no\n```", true, true},
		{"unparseable block", "Q1 - **Bad?**: x\n\n" + fence + "type: options\n```", true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := parseRoundSegment(1, c.seg)
			if (p.Err != nil) != c.wantBad {
				t.Fatalf("bad block = %v, want %v", p.Err, c.wantBad)
			}
			if (p.Card.Note != "") != c.wantNote {
				t.Fatalf("card note = %q, want note %v", p.Card.Note, c.wantNote)
			}
			if got := len(badRoundBlocks(grillround.Marker+strings.TrimPrefix(c.seg, "Q"))) == 1; got != c.wantBad {
				t.Fatalf("badRoundBlocks found one = %v, want %v", got, c.wantBad)
			}
		})
	}
}

func TestParseRoundSegment_BodyDropsTheHeader(t *testing.T) {
	cases := map[string]struct{ seg, title, body string }{
		"strict":     {"Q3** - **Blocking edges**: Should I publish?\n\n➡️ Yes.", "Blocking edges", "Should I publish?\n\n➡️ Yes."},
		"no colon":   {"Q1** **Store** Which one?", "Store", "Which one?"},
		"plain":      {"Q1 Which store? Pick one.", "Which store?", "Which store? Pick one."},
		"title only": {"Q2** - **Keep going?**", "Keep going?", ""},
		"lettered":   {"Q8b** - **Bedrock proof**: Extend it?", "Bedrock proof", "Extend it?"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			card := parseRoundSegment(1, c.seg).Card
			if card.Title != c.title || card.Body != c.body {
				t.Errorf("title, body = %q, %q; want %q, %q", card.Title, card.Body, c.title, c.body)
			}
		})
	}
}

func TestParseRoundSegment_KeepsTheNumberLetter(t *testing.T) {
	if got := parseRoundSegment(1, "Q8b** - **Bedrock proof**: Extend it?").Card; got.Label != "8b" {
		t.Errorf("label = %q; want %q", got.Label, "8b")
	}
}

func TestRoundCards_InlineMarkerStaysInItsQuestion(t *testing.T) {
	text := "❓ **Q8** - **Fixture**: Which one?\n\n" +
		"- a) Replay a round with `❓ **Q1**`, then the next line.\n- b) Fixed reply.\n\n" +
		"❓ **Q9** - **Packaging**: Where?"
	cards := roundCards(text)
	if len(cards) != 2 || cards[0].Label != "8" || cards[1].Label != "9" {
		t.Fatalf("cards = %+v; want Q8 and Q9", cards)
	}
	if !strings.Contains(cards[0].Body, "b) Fixed reply.") {
		t.Errorf("Q8 body = %q; want it to keep both options", cards[0].Body)
	}
	if grillround.HasMarker("Quote `❓ **Q1**` inline only.") {
		t.Error("an inline marker opened a round")
	}
}
