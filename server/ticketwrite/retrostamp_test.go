package ticketwrite

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// readFile stays here: this internal test cannot import servertest (it imports server).
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

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

func retroServer(store string) *Writer {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	set := &settings.Holder{}
	set.Store(&settings.Settings{})
	reg := projects.NewRegistry(func() projects.Env { return projects.Env{TicketStore: store} }, log, nil, set, nil, nil)
	return New(Deps{Log: log, Projects: reg})
}

func launchWriter(t *testing.T) (*Writer, *runs.Registry) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg, _, err := runs.Open(t.TempDir(), log)
	if err != nil {
		t.Fatal(err)
	}
	return New(Deps{Log: log, Registry: reg}), reg
}

// launchRun registers a run claimed at claimed and walks its launch through the
// four steps; the registry times them, so the stamp is checked for shape.
func launchRun(reg *runs.Registry, address string, claimed time.Time) {
	reg.Delete(address) // Put carries a replaced entry's timings over
	reg.Put(runs.TrackedRun{Run: runs.Run{Address: address}, StartedAt: claimed})
	for _, step := range []runs.LaunchStep{runs.StepCreatingWorktree, runs.StepInstallingDeps, runs.StepStartingAgent, runs.StepSendingPrompt} {
		reg.SetLaunchStep(address, step)
	}
	reg.EndLaunch(address)
}

func TestStampFinished_LaunchedTwiceHasTwoEntries(t *testing.T) {
	t.Parallel()
	_, addr, path, _ := retroFixture(t, "")
	w, reg := launchWriter(t)
	first := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	launchRun(reg, addr.String(), first)
	w.stampLaunch(addr, path)
	launchRun(reg, addr.String(), first.Add(time.Hour))
	w.stampLaunch(addr, path)
	launchRun(reg, addr.String(), first.Add(time.Hour))
	w.stampLaunch(addr, path) // the same claim finished again replaces its entry

	tk, err := schema.ParseTicket(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(tk.Launch) != 2 {
		t.Fatalf("launch = %+v, want two entries", tk.Launch)
	}
	for i, claimed := range []time.Time{first, first.Add(time.Hour)} {
		if got := tk.Launch[i].ClaimedAt; !got.Equal(claimed) {
			t.Errorf("entry %d claimed at %v, want %v", i, got, claimed)
		}
	}
}

func TestStampFinished_NoLaunchDataGetsNoStamp(t *testing.T) {
	t.Parallel()
	_, addr, path, _ := retroFixture(t, "")
	w, reg := launchWriter(t)
	w.stampLaunch(addr, path) // untracked
	reg.Put(runs.TrackedRun{Run: runs.Run{Address: addr.String()}, StartedAt: time.Now()})
	w.stampLaunch(addr, path) // never timed
	reg.Delete(addr.String())
	reg.Put(runs.TrackedRun{Run: runs.Run{Address: addr.String()}, StartedAt: time.Now()})
	reg.SetLaunchStep(addr.String(), runs.StepSendingPrompt)
	reg.SetLaunchStep(addr.String(), runs.StepCreatingWorktree)
	w.stampLaunch(addr, path) // launch still open

	tk, err := schema.ParseTicket(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(tk.Launch) != 0 {
		t.Errorf("launch = %+v, want none", tk.Launch)
	}
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
