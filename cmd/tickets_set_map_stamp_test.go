package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/tickets/schema"
)

// closeMapTicket records conversations against a map ticket created at
// createdAt, closes it with `set --status=done`, and returns the stored ticket.
func closeMapTicket(t *testing.T, createdAt time.Time, recs ...conversations.Record) schema.Ticket {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root, err := conversations.DefaultDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := conversations.MarkStarted(root, time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	epic := filepath.Join(t.TempDir(), "map-epic")
	if err := os.MkdirAll(filepath.Join(epic, "issues"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTicketFile(t, filepath.Join(epic, "ticket.md"), "---\nkind: map\nstatus: open\n---\n# Map\n")
	path := filepath.Join(epic, "issues", "04-pick.md")
	writeTicketFile(t, path, "---\nid: \"04\"\nstatus: claimed\ntype: grilling\ncreated_at: "+createdAt.UTC().Format(time.RFC3339)+"\n---\nBody.\n")
	for _, r := range recs {
		r.Ticket = ticketLabel(path)
		if err := conversations.Save(root, r); err != nil {
			t.Fatal(err)
		}
	}
	d := deps{stdout: bytes.NewBuffer(nil), stderr: bytes.NewBuffer(nil), getwd: nonAgentGetwd(t)}
	if err := execute([]string{"tickets", "set", path, "--status=done", "--commitless=true"}, d); err != nil {
		t.Fatal(err)
	}
	tk, err := schema.ParseTicket(path)
	if err != nil {
		t.Fatal(err)
	}
	return tk
}

func TestExecute_TicketsSet_MapDoneStampsConversationTotals(t *testing.T) {
	tk := closeMapTicket(t, time.Now(),
		conversations.Record{ID: "a", Totals: &conversations.Totals{TotalTokens: 100, Cost: 0.5}},
		conversations.Record{ID: "b", Totals: &conversations.Totals{TotalTokens: 20, Cost: 0.25}},
	)
	if tk.TotalTokens == nil || *tk.TotalTokens != 120 || tk.ActualCost != 0.75 {
		t.Errorf("tokens = %v, cost = %v; want 120 and 0.75", tk.TotalTokens, tk.ActualCost)
	}
}

func TestExecute_TicketsSet_MapDoneWithoutConversationsStampsZero(t *testing.T) {
	tk := closeMapTicket(t, time.Now())
	if tk.TotalTokens == nil || *tk.TotalTokens != 0 {
		t.Errorf("tokens = %v; want a stamped 0", tk.TotalTokens)
	}
}

func TestExecute_TicketsSet_MapDoneMissingTotalsStampsNothing(t *testing.T) {
	tk := closeMapTicket(t, time.Now(),
		conversations.Record{ID: "a", Totals: &conversations.Totals{TotalTokens: 100, Cost: 0.5}},
		conversations.Record{ID: "b"},
	)
	if tk.TotalTokens != nil || tk.ActualCost != 0 {
		t.Errorf("tokens = %v, cost = %v; want nothing stamped", tk.TotalTokens, tk.ActualCost)
	}
}

func TestExecute_TicketsSet_MapDonePreRecordingTicketStampsNothing(t *testing.T) {
	tk := closeMapTicket(t, time.Now().Add(-24*time.Hour),
		conversations.Record{ID: "a", Totals: &conversations.Totals{TotalTokens: 100, Cost: 0.5}},
	)
	if tk.TotalTokens != nil {
		t.Errorf("tokens = %v; want nothing stamped", tk.TotalTokens)
	}
}
