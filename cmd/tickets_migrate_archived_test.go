package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

func TestExecute_TicketsMigrate_BackfillsArchivedEpics(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "gx")
	archive := filepath.Join(root, ".archive")
	const done = "---\nid: \"01\"\nstatus: done\ntype: implement\n---\nBody.\n"
	writeMigrateFixtureEpic(t, archive, "old", map[string]string{"01-a.md": done})
	testutil.WriteFile(t, filepath.Join(archive, "old"), "ticket.md", "---\nstatus: done\nmerged_at: 2026-01-02T03:04:05Z\n---\n")
	writeRunLog(t, filepath.Join(archive, "old"), []string{
		`{"time":"2026-01-01T00:01:00Z","type":"conflict-hit","ticket":"01"}`,
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

	epics, err := tickets.LoadArchived(root)
	if err != nil || len(epics) != 1 {
		t.Fatalf("LoadArchived = %d epics, err %v", len(epics), err)
	}
	epic := epics[0]
	tk := epic.Tickets[0]
	if tk.Friction == nil || tk.Friction.RunEvents["conflict-hit"] != 1 {
		t.Errorf("archived ticket friction = %+v, want its conflict-hit event", tk.Friction)
	}
	if tk.TotalTokens == nil {
		t.Errorf("archived ticket total_tokens not stamped")
	}
	if epic.Friction == nil || epic.Friction.RunEvents["conflict-hit"] != 1 {
		t.Errorf("archived epic friction = %+v, want the sum of its tickets", epic.Friction)
	}
	if epic.TotalTokens == nil {
		t.Errorf("archived epic total_tokens not stamped")
	}

	if again := run(); !strings.Contains(again, "no changes") {
		t.Errorf("second run changed something:\n%s", again)
	}
}
