package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func TestExecute_TicketsMigrate_RewritesOldStatusesToPaused(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	issuesDir := writeMigrateFixtureEpic(t, root, "widget-epic", map[string]string{
		"01-answer-kind.md":    "---\nid: \"01\"\nstatus: needs-answer\ntype: implement\npark_kind: self-reported\n---\nBody.\n",
		"02-answer-none.md":    "---\nid: \"02\"\nstatus: needs-answer\ntype: implement\n---\nBody.\n",
		"03-repair-kind.md":    "---\nid: \"03\"\nstatus: needs-repair\ntype: implement\npark_kind: land-conflict\n---\nBody.\n",
		"04-repair-log.md":     "---\nid: \"04\"\nstatus: needs-repair\ntype: implement\n---\nBody.\n",
		"05-repair-none.md":    "---\nid: \"05\"\nstatus: needs-repair\ntype: implement\n---\nBody.\n",
		"06-mismatch.md":       "---\nid: \"06\"\nstatus: needs-answer\ntype: implement\npark_kind: spinning\n---\nBody.\n",
		"07-repair-heading.md": "---\nid: \"07\"\nstatus: needs-repair\ntype: implement\n---\nBody.\n\n## Needs Repair\n\nIt broke.\n",
		"08-broken.md":         "no frontmatter at all\n",
	})
	path := func(name string) string { return filepath.Join(issuesDir, name) }
	if _, err := tickets.AppendLog(path("04-repair-log.md"), tickets.LogEvent{Type: tickets.EventPauseQuestion, By: "gx", PauseKind: "spinning", Summary: "stuck"}); err != nil {
		t.Fatal(err)
	}

	mapIssues := writeMigrateFixtureEpic(t, root, "map-epic", map[string]string{
		"01-map.md": "---\nid: \"01\"\nstatus: needs-answer\ntype: implement\npark_kind: self-reported\n---\nBody.\n",
	})
	if err := os.WriteFile(filepath.Join(root, "map-epic", "ticket.md"), []byte("---\nkind: map\n---\n# Map\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	run := func(args ...string) string {
		t.Helper()
		var stdout bytes.Buffer
		argv := append([]string{"tickets", "migrate", root}, args...)
		if err := execute(argv, deps{stdout: &stdout, stderr: &bytes.Buffer{}}); err != nil {
			t.Fatalf("execute %v: %v", argv, err)
		}
		return stdout.String()
	}

	dry := run("--dry-run")
	for _, want := range []string{
		"01-answer-kind.md: status -> paused (pause_reason: self-reported)",
		"02-answer-none.md: status -> paused (pause_reason: zero-commit)",
		"03-repair-kind.md: status -> paused (pause_reason: merge-conflict)",
		"04-repair-log.md: status -> paused (pause_reason: spinning)",
		"05-repair-none.md: status -> paused (pause_reason: iteration-error)",
		"06-mismatch.md: status -> paused (pause_reason: zero-commit); recorded reason \"spinning\" does not fit status needs-answer",
	} {
		if !strings.Contains(dry, want) {
			t.Errorf("dry run missing %q:\n%s", want, dry)
		}
	}
	if strings.Contains(dry, "01-map.md") || strings.Contains(dry, "08-broken.md") {
		t.Errorf("dry run lists a skipped ticket:\n%s", dry)
	}
	if b, _ := os.ReadFile(path("02-answer-none.md")); !strings.Contains(string(b), "needs-answer") {
		t.Errorf("dry run wrote a file:\n%s", b)
	}
	lines := strings.Split(strings.TrimSpace(dry), "\n")
	listed := 0
	for _, l := range lines {
		if strings.Contains(l, ".md: ") {
			listed++
		}
	}
	if last := lines[len(lines)-1]; last != fmt.Sprintf("%d file(s) would change", listed) {
		t.Errorf("count line %q does not match %d listed files:\n%s", last, listed, dry)
	}
	for _, name := range []string{"01-answer-kind.md", "02-answer-none.md"} {
		if n := strings.Count(dry, name+":"); n != 1 {
			t.Errorf("dry run lists %s %d times, want once:\n%s", name, n, dry)
		}
	}

	if err := os.Remove(path("08-broken.md")); err != nil {
		t.Fatal(err)
	}
	run()

	want := map[string]schema.PauseReason{
		"01-answer-kind.md":    schema.PauseSelfReported,
		"02-answer-none.md":    schema.PauseZeroCommit,
		"03-repair-kind.md":    schema.PauseMergeConflict,
		"04-repair-log.md":     schema.PauseSpinning,
		"05-repair-none.md":    schema.PauseIterationError,
		"06-mismatch.md":       schema.PauseZeroCommit,
		"07-repair-heading.md": schema.PauseIterationError,
	}
	for name, reason := range want {
		raw, err := os.ReadFile(path(name))
		if err != nil {
			t.Fatal(err)
		}
		got, err := schema.ParseTicketFromRawUnshimmed(string(raw), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got.Status != schema.StatusPaused || got.PauseReason != reason || got.PauseKind != "" {
			t.Errorf("%s = status %q reason %q park_kind %q, want paused %q", name, got.Status, got.PauseReason, got.PauseKind, reason)
		}
		if strings.Contains(string(raw), "park_kind") {
			t.Errorf("%s still has park_kind:\n%s", name, raw)
		}
	}
	heading, _ := os.ReadFile(path("07-repair-heading.md"))
	if !strings.Contains(string(heading), "\n## Paused\n") || strings.Contains(string(heading), "## Needs Repair") {
		t.Errorf("heading not rewritten:\n%s", heading)
	}
	if mapAfter, _ := os.ReadFile(filepath.Join(mapIssues, "01-map.md")); !strings.Contains(string(mapAfter), "status: needs-answer") || strings.Contains(string(mapAfter), "pause_reason") {
		t.Errorf("map-epic ticket was paused:\n%s", mapAfter)
	}

	before, _ := os.ReadFile(path("07-repair-heading.md"))
	if out := strings.TrimSpace(run()); out != "no changes" {
		t.Errorf("second run stdout = %q, want no changes", out)
	}
	if after, _ := os.ReadFile(path("07-repair-heading.md")); !bytes.Equal(before, after) {
		t.Error("ticket changed on second run")
	}
}
