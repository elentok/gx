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

func stampedDone(id, tokens, cost string) string {
	return "---\nid: \"" + id + "\"\nstatus: done\ntype: implement\ntotal_tokens: " + tokens + "\nactual_cost: " + cost + "\n---\nBody.\n"
}

func commitTrailer(t *testing.T, dir, name, content, trailerValue string) {
	t.Helper()
	testutil.WriteFile(t, dir, name, content)
	testutil.MustGitExported(t, dir, "add", ".")
	testutil.MustGitExported(t, dir, "commit", "-m", "add "+name+"\n\nRalph-Loop-Ticket: "+trailerValue)
}

func TestExecute_TicketsMigrate_BackfillsEpicLinesTokensAndCost(t *testing.T) {
	t.Parallel()
	repoDir, _ := mergeTestRepo(t, map[string]string{"full": "full", "partial": "partial"})
	commitTrailer(t, filepath.Join(repoDir, "full"), "a.txt", "1\n2\n3\n", "full/01")
	commitTrailer(t, filepath.Join(repoDir, "full"), "b.txt", "1\n2\n", "full/02")
	commitFile(t, filepath.Join(repoDir, "full"), "untrailered.txt", "1\n2\n3\n4\n5\n")
	commitTrailer(t, filepath.Join(repoDir, "partial"), "c.txt", "1\n", "partial/01")

	root := t.TempDir()
	testutil.WriteFile(t, root, "project.json", `{"repo": "`+repoDir+`", "trunk": "main"}`)
	const merged = "completed_at: 2026-01-02T03:04:05Z\nmerged_at: 2026-01-02T03:04:05Z\n"
	writeEpic := func(name string, issues map[string]string) {
		writeMigrateFixtureEpic(t, root, name, issues)
		testutil.WriteFile(t, filepath.Join(root, name), "ticket.md", "---\nstatus: done\n"+merged+"---\n")
	}
	writeEpic("full", map[string]string{
		"01-a.md": stampedDone("01", "100", "0.5"),
		"02-b.md": stampedDone("02", "200", "1.25"),
	})
	writeEpic("partial", map[string]string{
		"01-a.md": stampedDone("01", "100", "0.5"),
		"02-b.md": "---\nid: \"02\"\nstatus: done\ntype: implement\n---\nBody.\n",
	})

	run := func() string {
		t.Helper()
		var stdout bytes.Buffer
		if err := execute([]string{"tickets", "migrate", root}, deps{stdout: &stdout, stderr: &bytes.Buffer{}, env: envFor(t)}); err != nil {
			t.Fatalf("execute: %v", err)
		}
		return stdout.String()
	}
	run()

	full := tickets.LoadEpicTicketMD(filepath.Join(root, "full"))
	if full.LinesAdded == nil || *full.LinesAdded != 5 || full.LinesRemoved == nil || *full.LinesRemoved != 0 {
		t.Errorf("full lines = %v/%v, want 5/0", full.LinesAdded, full.LinesRemoved)
	}
	if full.TotalTokens == nil || *full.TotalTokens != 300 || full.ActualCost != 1.75 {
		t.Errorf("full tokens/cost = %v/%v, want 300/1.75", full.TotalTokens, full.ActualCost)
	}

	partial := tickets.LoadEpicTicketMD(filepath.Join(root, "partial"))
	if partial.LinesAdded == nil || *partial.LinesAdded != 1 {
		t.Errorf("partial lines added = %v, want 1", partial.LinesAdded)
	}
	if partial.TotalTokens != nil || partial.ActualCost != 0 {
		t.Errorf("partial has tokens/cost %v/%v despite an unstamped done ticket", partial.TotalTokens, partial.ActualCost)
	}

	before, err := os.ReadFile(filepath.Join(root, "full", "ticket.md"))
	if err != nil {
		t.Fatal(err)
	}
	if again := run(); !strings.Contains(again, "no changes") {
		t.Errorf("second run changed something:\n%s", again)
	}
	after, err := os.ReadFile(filepath.Join(root, "full", "ticket.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("rerun rewrote full/ticket.md:\n%s\nvs\n%s", before, after)
	}
}
