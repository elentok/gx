package tickets

import (
	"testing"

	"github.com/elentok/gx/tickets"
)

func loadSingleTicket(t *testing.T, root, epicName, filename, content string) (tickets.Epic, tickets.Ticket) {
	t.Helper()
	writeTicket(t, root, epicName, filename, content)
	epics, err := tickets.Load(root + "/.scratch")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range epics {
		if e.Name != epicName {
			continue
		}
		for _, ticket := range e.Tickets {
			if ticket.Path == ticketPath(root, epicName, filename) {
				return e, ticket
			}
		}
	}
	t.Fatalf("ticket %s/%s not found after load", epicName, filename)
	return tickets.Epic{}, tickets.Ticket{}
}

func TestParkReason_NeedsAnswerFirstNonEmptyLineStripped(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	epic, ticket := loadSingleTicket(t, root, "my-epic", "01-first.md",
		"Status: needs-answer\n\n## Needs Answer\n\n**Which approach?** option A or B\n")

	got := parkReason(epic, ticket, "...")
	want := "Which approach? option A or B"
	if got != want {
		t.Fatalf("parkReason() = %q, want %q", got, want)
	}
}

func TestParkReason_NeedsRepairFirstNonEmptyLineStripped(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	epic, ticket := loadSingleTicket(t, root, "my-epic", "01-first.md",
		"Status: needs-repair\n\n## Needs Repair\n\n_build failed_ with exit 1\n")

	got := parkReason(epic, ticket, "...")
	want := "build failed with exit 1"
	if got != want {
		t.Fatalf("parkReason() = %q, want %q", got, want)
	}
}

// TestParkReason_StaleSectionStillGatedParked covers the "gated on parked
// status, not on section presence" acceptance criterion: a needs-answer
// ticket whose section has already been retired (demoted elsewhere) has no
// heading to find, so parkReason returns "" — but the row stays classified
// as parked by RenderedStatus regardless.
func TestParkReason_StaleSectionStillGatedParked(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	epic, ticket := loadSingleTicket(t, root, "my-epic", "01-first.md",
		"Status: needs-answer\n\nNo park section here.\n")

	if got := parkReason(epic, ticket, "..."); got != "" {
		t.Fatalf("parkReason() = %q, want empty for a retired/missing section", got)
	}
	if status := epic.RenderedStatus(ticket); status != tickets.StatusNeedsAnswer {
		t.Fatalf("RenderedStatus() = %v, want StatusNeedsAnswer regardless of missing section", status)
	}
}

func TestParkReason_DraftNotParked(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	epic, ticket := loadSingleTicket(t, root, "my-epic", "01-first.md",
		"Status: draft\n\n## Needs Answer\n\nSomething.\n")

	if got := parkReason(epic, ticket, "..."); got != "" {
		t.Fatalf("parkReason() = %q, want empty for draft status (out of scope)", got)
	}
}

func TestParkReason_OpenStatusIgnoresLeftoverSection(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	epic, ticket := loadSingleTicket(t, root, "my-epic", "01-first.md",
		"Status: open\n\n## Needs Answer\n\nSomething.\n")

	if got := parkReason(epic, ticket, "..."); got != "" {
		t.Fatalf("parkReason() = %q, want empty for non-parked status", got)
	}
}

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
