package tickets

import (
	"strings"
	"testing"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
)

func TestEllipsize_TruncatesAtRuneCapWithIcon(t *testing.T) {
	t.Parallel()
	long := ""
	for range 100 {
		long += "x"
	}
	got := ellipsize(long, 60, "…")
	wantLen := 60 + len([]rune("…"))
	if got := []rune(got); len(got) != wantLen {
		t.Fatalf("ellipsize() len = %d, want %d", len(got), wantLen)
	}
	if got := ellipsize("short", 60, "…"); got != "short" {
		t.Fatalf("ellipsize() = %q, want unchanged short text", got)
	}
}

func TestRowTitle_EllipsizesLongHeadingTitle(t *testing.T) {
	t.Parallel()
	icons := ui.Icons(false)
	long := tickets.Ticket{Identifier: "13", Number: 13, Title: "TUI Watch agent modal, hidden herdr features, non-interactive investigate"}
	got := rowTitle(icons, long)
	if !strings.HasPrefix(got, "13 TUI Watch") || !strings.HasSuffix(got, icons.Ellipsis) {
		t.Fatalf("rowTitle() = %q, want number + capped title + ellipsis", got)
	}
	short := tickets.Ticket{Identifier: "02", Number: 2, Title: "OS notifier package"}
	if got := rowTitle(icons, short); !strings.HasSuffix(got, " OS notifier package") {
		t.Fatalf("rowTitle() = %q, want short title unchanged", got)
	}
}
