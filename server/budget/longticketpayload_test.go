package budget_test

import (
	"encoding/json"
	"github.com/elentok/gx/server/servertest/rigs"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

func fetchDetail(t *testing.T, h *servertest.Harness, base, address string) wire.TicketDetail {
	t.Helper()
	code, body := servertest.WebDo(t, h, http.MethodGet, base+"/v1/tickets/detail?address="+address)
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	var d wire.TicketDetail
	if err := json.Unmarshal([]byte(body), &d); err != nil {
		t.Fatalf("body %q: %v", body, err)
	}
	return d
}

// withRepo gives the project a repo; findTicket refuses a project without one.
func withRepo(t *testing.T, store string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(store, "proj", "project.json"), []byte(`{"name":"proj","repo":"/x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The Long-running badge reads the mark and the current threshold from the epic row and the ticket
// detail; an unmarked ticket carries neither. (The alert that writes the mark is in servertest.)
func TestLongTicketMark_ReachesTheEpicRowAndTheTicketDetail(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	rigs.WriteEpicFiles(t, store, "work", "---\nstatus: open\n---\n", map[string]string{
		"01-a.md": rigs.IssueMD("01", "open", "long_ticket_turns: 120\n", "Marked"),
		"02-b.md": rigs.IssueMD("02", "open", "", "Plain"),
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

	_, epic, _ := rigs.GetEpicDetail(t, h, base, "proj:work")
	if r := rigs.RowOf(t, epic, "proj:work/01"); r.LongTicketTurns != 120 || r.LongTicketThreshold != 100 {
		t.Errorf("marked row = %d turns, threshold %d, want 120 and 100", r.LongTicketTurns, r.LongTicketThreshold)
	}
	if r := rigs.RowOf(t, epic, "proj:work/02"); r.LongTicketTurns != 0 || r.LongTicketThreshold != 0 {
		t.Errorf("plain row = %d turns, threshold %d, want none", r.LongTicketTurns, r.LongTicketThreshold)
	}

	if d := fetchDetail(t, h, base, "proj:work/01"); d.LongTicketTurns != 120 || d.LongTicketThreshold != 100 {
		t.Errorf("marked detail = %d turns, threshold %d, want 120 and 100", d.LongTicketTurns, d.LongTicketThreshold)
	}
	if d := fetchDetail(t, h, base, "proj:work/02"); d.LongTicketTurns != 0 || d.LongTicketThreshold != 0 {
		t.Errorf("plain detail = %d turns, threshold %d, want none", d.LongTicketTurns, d.LongTicketThreshold)
	}
}
