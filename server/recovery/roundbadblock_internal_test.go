package recovery

import (
	"strings"
	"testing"

	"github.com/elentok/gx/grillround"
	"github.com/elentok/gx/server/sessions/chat"
)

func TestParseRoundSegment_BadBlockAndCleanCardAreExclusive(t *testing.T) {
	t.Parallel()
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
			p := chat.ParseRoundSegment(1, c.seg)
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
