package chat

import (
	"strings"
	"testing"

	"github.com/elentok/gx/grillround"
)

func TestParseRoundSegment_BodyDropsTheHeader(t *testing.T) {
	t.Parallel()
	cases := map[string]struct{ seg, title, body string }{
		"strict":     {"Q3** - **Blocking edges**: Should I publish?\n\n➡️ Yes.", "Blocking edges", "Should I publish?\n\n➡️ Yes."},
		"no colon":   {"Q1** **Store** Which one?", "Store", "Which one?"},
		"plain":      {"Q1 Which store? Pick one.", "Which store?", "Which store? Pick one."},
		"title only": {"Q2** - **Keep going?**", "Keep going?", ""},
		"lettered":   {"Q8b** - **Bedrock proof**: Extend it?", "Bedrock proof", "Extend it?"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			card := ParseRoundSegment(1, c.seg).Card
			if card.Title != c.title || card.Body != c.body {
				t.Errorf("title, body = %q, %q; want %q, %q", card.Title, card.Body, c.title, c.body)
			}
		})
	}
}

func TestParseRoundSegment_KeepsTheNumberLetter(t *testing.T) {
	t.Parallel()
	if got := ParseRoundSegment(1, "Q8b** - **Bedrock proof**: Extend it?").Card; got.Label != "8b" {
		t.Errorf("label = %q; want %q", got.Label, "8b")
	}
}

func TestRoundCards_InlineMarkerStaysInItsQuestion(t *testing.T) {
	t.Parallel()
	text := "❓ **Q8** - **Fixture**: Which one?\n\n" +
		"- a) Replay a round with `❓ **Q1**`, then the next line.\n- b) Fixed reply.\n\n" +
		"❓ **Q9** - **Packaging**: Where?"
	cards := RoundCards(text)
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
