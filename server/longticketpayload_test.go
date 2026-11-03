package server_test

import (
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

// The Long-running badge reads the mark and the current threshold from the epic row and the ticket
// detail; an unmarked ticket carries neither. (The alert that writes the mark is in servertest.)
func TestLongTicketMark_ReachesTheEpicRowAndTheTicketDetail(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	writeEpicFiles(t, store, "work", "---\nstatus: open\n---\n", map[string]string{
		"01-a.md": issueMD("01", "open", "long_ticket_turns: 120\n", "Marked"),
		"02-b.md": issueMD("02", "open", "", "Plain"),
	})
	withRepo(t, store)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.WebAddr = "127.0.0.1:0"
		c.LongTicketTurns = 100
	})
	hs, err := h.Client.Handshake(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	base := hs.WebURL

	_, epic, _ := getEpicDetail(t, h, base, "proj:work")
	if r := rowOf(t, epic, "proj:work/01"); r.LongTicketTurns != 120 || r.LongTicketThreshold != 100 {
		t.Errorf("marked row = %d turns, threshold %d, want 120 and 100", r.LongTicketTurns, r.LongTicketThreshold)
	}
	if r := rowOf(t, epic, "proj:work/02"); r.LongTicketTurns != 0 || r.LongTicketThreshold != 0 {
		t.Errorf("plain row = %d turns, threshold %d, want none", r.LongTicketTurns, r.LongTicketThreshold)
	}

	if d := fetchDetail(t, h, base, "proj:work/01"); d.LongTicketTurns != 120 || d.LongTicketThreshold != 100 {
		t.Errorf("marked detail = %d turns, threshold %d, want 120 and 100", d.LongTicketTurns, d.LongTicketThreshold)
	}
	if d := fetchDetail(t, h, base, "proj:work/02"); d.LongTicketTurns != 0 || d.LongTicketThreshold != 0 {
		t.Errorf("plain detail = %d turns, threshold %d, want none", d.LongTicketTurns, d.LongTicketThreshold)
	}
}
