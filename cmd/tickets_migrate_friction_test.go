package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

func TestExecute_TicketsMigrate_BackfillsFrictionSummaries(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	root := filepath.Join(store, "gx")
	const done = "---\nid: \"01\"\nstatus: done\ntype: implement\n---\nBody.\n"
	writeMigrateFixtureEpic(t, root, "late", map[string]string{"01-a.md": done, "02-b.md": strings.Replace(done, `"01"`, `"02"`, 1)})
	writeMigrateFixtureEpic(t, root, "early", map[string]string{"01-a.md": done})
	testutil.WriteFile(t, filepath.Join(root, "late"), "ticket.md", "---\nstatus: done\nmerged_at: 2026-01-02T03:04:05Z\n---\n")
	writeRunLog(t, filepath.Join(root, "late"), []string{
		`{"time":"2026-01-01T00:00:00Z","type":"paused","ticket":"01","kind":"conflict"}`,
		`{"time":"2026-01-01T00:00:01Z","type":"paused","ticket":"01","kind":"conflict"}`,
		`{"time":"2026-01-01T00:01:00Z","type":"conflict-hit","ticket":"02"}`,
	})
	// An early run log sits in the store root, beside the project directories.
	writeRunLog(t, filepath.Join(store, "early"), []string{
		`{"time":"2026-01-01T00:00:00Z","type":"conflict-hit","ticket":"01"}`,
		`{"time":"2026-01-01T00:00:01Z","type":"conflict-hit","ticket":"01"}`,
	})

	run := func() string {
		t.Helper()
		var stdout bytes.Buffer
		if err := execute([]string{"tickets", "migrate", root}, deps{stdout: &stdout, stderr: &bytes.Buffer{}}); err != nil {
			t.Fatalf("execute: %v", err)
		}
		return stdout.String()
	}
	run()

	epics, err := tickets.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]tickets.Epic{}
	for _, e := range epics {
		byName[e.Name] = e
	}

	late1 := byName["late"].Tickets[0].Friction
	if late1 == nil || late1.PauseEndings["unknown"] != 1 || late1.PausesByReason["conflict"] != 1 {
		t.Errorf("past ticket friction = %+v, want one pause with an unknown ending", late1)
	}
	if got := byName["early"].Tickets[0].Friction; got == nil || got.RunEvents["conflict-hit"] != 2 {
		t.Errorf("early store-root ticket friction = %+v, want its 2 conflict-hit events", got)
	}
	sum := byName["late"].Friction
	if sum == nil || sum.PauseEndings["unknown"] != 1 || sum.RunEvents["conflict-hit"] != 1 {
		t.Errorf("merged epic friction = %+v, want the sum of its tickets", sum)
	}

	if again := run(); !strings.Contains(again, "no changes") {
		t.Errorf("second run changed something:\n%s", again)
	}
	if _, err := os.Stat(filepath.Join(store, "early", "ticket.md")); err == nil {
		t.Errorf("migrate wrote into the store-root epic directory")
	}
}
