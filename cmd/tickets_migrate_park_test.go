package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/tickets"
)

func TestExecute_TicketsMigrate_MovesNeedsAnswerIntoLog(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	sections := "\n## Needs Answer\n\nWhich store?\n- a\n- b\n\n## Handoff\n\nResume at step 3.\n"
	issuesDir := writeMigrateFixtureEpic(t, root, "widget-epic", map[string]string{
		"01-parked.md":   "---\nid: \"01\"\nstatus: needs-answer\ntype: implement\npark_kind: self-reported\n---\nBody.\n" + sections,
		"02-answered.md": "---\nid: \"02\"\nstatus: open\ntype: implement\n---\nBody.\n" + sections,
		"03-plain.md":    "---\nid: \"03\"\nstatus: open\ntype: implement\n---\nBody.\n",
	})
	parked := filepath.Join(issuesDir, "01-parked.md")
	answered := filepath.Join(issuesDir, "02-answered.md")

	run := func() string {
		var stdout bytes.Buffer
		if err := execute([]string{"tickets", "migrate", root}, deps{stdout: &stdout, stderr: &bytes.Buffer{}}); err != nil {
			t.Fatalf("execute tickets migrate: %v", err)
		}
		return stdout.String()
	}
	run()

	events, err := tickets.ReadLog(parked)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("parked log has %d events, want 1: %+v", len(events), events)
	}
	q := events[0]
	if q.Type != tickets.EventParkQuestion || q.By != "agent" || q.ParkKind != "self-reported" ||
		q.Summary != "Which store?" || q.Handoff != "Resume at step 3." {
		t.Errorf("question = %+v", q)
	}
	if tickets.OpenQuestion(events) == nil {
		t.Error("parked ticket has no open question")
	}

	events, err = tickets.ReadLog(answered)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].ParkKind != "zero-commit" ||
		events[1].Type != tickets.EventParkAnswer || events[1].Question != events[0].Seq {
		t.Fatalf("answered log = %+v, want question (zero-commit) then its answer", events)
	}
	if tickets.OpenQuestion(events) != nil {
		t.Error("answered ticket still has an open question")
	}

	for _, p := range []string{parked, answered} {
		b, _ := os.ReadFile(p)
		if strings.Contains(string(b), "\n## Needs Answer") || strings.Contains(string(b), "\n## Handoff") {
			t.Errorf("%s still has live sections:\n%s", p, b)
		}
		if !strings.Contains(string(b), "retired from `## Needs Answer`") {
			t.Errorf("%s has no retired comment:\n%s", p, b)
		}
	}
	if _, err := os.Stat(tickets.LogPath(filepath.Join(issuesDir, "03-plain.md"))); !os.IsNotExist(err) {
		t.Errorf("plain ticket got a log: %v", err)
	}

	before, _ := os.ReadFile(parked)
	if out := strings.TrimSpace(run()); out != "no changes" {
		t.Errorf("second run stdout = %q, want no changes", out)
	}
	after, _ := os.ReadFile(parked)
	if !bytes.Equal(before, after) {
		t.Error("parked ticket changed on second run")
	}
	if events, _ := tickets.ReadLog(parked); len(events) != 1 {
		t.Errorf("second run grew the log to %d events", len(events))
	}
}
