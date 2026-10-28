package server

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// retroFixture writes an epic with an extra frontmatter key and an evolve
// one-off targeting it.
func retroFixture(t *testing.T, epicFM string) (store string, addr tickets.Address, ticketPath, epicTicket string) {
	t.Helper()
	store = t.TempDir()
	if err := os.MkdirAll(filepath.Join(store, "proj"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "proj", "project.json"), []byte(`{"name":"proj"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	epicPath := filepath.Join(store, "proj", "epic-a")
	if err := os.MkdirAll(epicPath, 0o755); err != nil {
		t.Fatal(err)
	}
	epicTicket = filepath.Join(epicPath, "ticket.md")
	if err := os.WriteFile(epicTicket, []byte("---\nstatus: open\nlabel: keep-me\n"+epicFM+"---\n\n# epic\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oneOff := filepath.Join(store, "proj", "evolve-epic-a-20261010-1405")
	testutil.EnsureEpicTicketMD(t, oneOff)
	if err := os.MkdirAll(filepath.Join(oneOff, "issues"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := schema.MarshalTicket(schema.Ticket{ID: "01", Status: schema.StatusOpen, Type: schema.TypeEvolve, Unique: "evolve:proj/epic-a"}, "# retro\n")
	if err != nil {
		t.Fatal(err)
	}
	ticketPath = filepath.Join(oneOff, "issues", "01-retro.md")
	if err := os.WriteFile(ticketPath, out, 0o644); err != nil {
		t.Fatal(err)
	}
	return store, tickets.Address{Project: "proj", Epic: "evolve-epic-a-20261010-1405", ID: "01"}, ticketPath, epicTicket
}

func retroServer(store string) *Server {
	s := &Server{cfg: Config{TicketStore: store}, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	s.settings.Store(&Settings{})
	return s
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestStampRetro_DoneStampsEvolvedAtAndTicketKeepingOtherKeys(t *testing.T) {
	t.Parallel()
	store, addr, path, epicTicket := retroFixture(t, "")
	retroServer(store).stampRetro(addr, path, false)

	got := readFile(t, epicTicket)
	for _, want := range []string{"label: keep-me", "evolved_at:", "evolve_ticket: proj:evolve-epic-a-20261010-1405/01", "# epic"} {
		if !strings.Contains(got, want) {
			t.Errorf("epic lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "evolve_skipped") {
		t.Errorf("done retro stamped evolve_skipped:\n%s", got)
	}
}

func TestStampRetro_ForcedRerunOverwritesEvolveTicket(t *testing.T) {
	t.Parallel()
	store, addr, path, epicTicket := retroFixture(t, "evolved_at: 2026-01-02T03:04:05Z\nevolve_ticket: proj:old/01\n")
	retroServer(store).stampRetro(addr, path, false)

	got := readFile(t, epicTicket)
	if strings.Contains(got, "proj:old/01") || !strings.Contains(got, "evolve_ticket: proj:evolve-epic-a-20261010-1405/01") {
		t.Errorf("evolve_ticket not overwritten:\n%s", got)
	}
	if strings.Contains(got, "2026-01-02") {
		t.Errorf("evolved_at not refreshed:\n%s", got)
	}
}

func TestStampRetro_CancelledStampsEvolvedAtAndSkipped(t *testing.T) {
	t.Parallel()
	store, addr, path, epicTicket := retroFixture(t, "")
	retroServer(store).stampRetro(addr, path, true)

	got := readFile(t, epicTicket)
	for _, want := range []string{"label: keep-me", "evolved_at:", "evolve_skipped: true"} {
		if !strings.Contains(got, want) {
			t.Errorf("epic lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "evolve_ticket") {
		t.Errorf("cancelled retro stamped evolve_ticket:\n%s", got)
	}
}

func TestStampRetro_NonRetroTicketLeavesTheEpicAlone(t *testing.T) {
	t.Parallel()
	store, addr, path, epicTicket := retroFixture(t, "")
	out, err := schema.MarshalTicket(schema.Ticket{ID: "01", Status: schema.StatusOpen, Type: schema.TypeEvolve}, "# not a retro\n")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
	before := readFile(t, epicTicket)
	retroServer(store).stampRetro(addr, path, false)
	if got := readFile(t, epicTicket); got != before {
		t.Errorf("epic changed:\n%s", got)
	}
}
