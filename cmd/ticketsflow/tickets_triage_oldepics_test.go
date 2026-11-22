package ticketsflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func writeOldEpic(t *testing.T, project, name, front, notes string, ticketStatuses ...string) {
	t.Helper()
	body := "---\nkind: map\n" + front + "---\n\n# " + name + "\n"
	if notes != "" {
		body += "\n## Notes\n\n" + notes + "\n"
	}
	if err := os.MkdirAll(filepath.Join(project, name, "issues"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTicketFile(t, filepath.Join(project, name, "ticket.md"), body)
	for i, st := range ticketStatuses {
		id := string(rune('1'+i)) + "0"
		writeTicketFile(t, filepath.Join(project, name, "issues", id+"-t.md"),
			"---\nid: \""+id+"\"\nstatus: "+st+"\ntype: research\n---\n\n# T"+id+"\n")
	}
}

type oldEpicShown struct {
	Epic    string   `json:"epic"`
	Backlog string   `json:"backlog"`
	Move    []string `json:"move"`
	Archive bool     `json:"archive"`
	Waiting bool     `json:"waiting"`
	Claimed []string `json:"claimed"`
}

func oldEpics(t *testing.T, repo string) []oldEpicShown {
	t.Helper()
	out, err := runIn(t, repo, "tickets", "triage", "oldepics", "--json")
	if err != nil {
		t.Fatalf("oldepics: %v", err)
	}
	var got []oldEpicShown
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("oldepics json %q: %v", out, err)
	}
	return got
}

const triagedNote = "Triaged from follow-ups on 2026-10-01: gx:follow-ups/01"

func TestTriageOldEpics_ListsOnlyChartingUnarchivedNotedMaps(t *testing.T) {
	repo, project := triageProjectDir(t)
	writeOldEpic(t, project, "alpha", "", triagedNote, "open", "draft", "done")
	writeOldEpic(t, project, "beta", "design_step: chart\n", triagedNote, "open")
	writeOldEpic(t, project, "past", "design_step: resolve-tickets\n", triagedNote, "open")
	writeOldEpic(t, project, "plain", "", "Something else", "open")
	writeOldEpic(t, project, "start-design-map", "", "", "open")
	writeOldEpic(t, project, ".archive/gone", "", triagedNote, "open")

	var names []string
	for _, e := range oldEpics(t, repo) {
		names = append(names, e.Epic)
	}
	if want := []string{"alpha", "beta"}; !slices.Equal(names, want) {
		t.Fatalf("candidates = %v, want %v", names, want)
	}
}

func TestTriageOldEpics_ShowsMoveArchiveAndWaiting(t *testing.T) {
	repo, project := triageProjectDir(t)
	writeOldEpic(t, project, "movable", "", triagedNote, "open", "draft", "done", "cancelled")
	writeOldEpic(t, project, "mixed", "", triagedNote, "open", "claimed")
	writeOldEpic(t, project, "stuck", "", triagedNote, "claimed", "done")
	writeOldEpic(t, project, "doneonly", "", triagedNote, "done")
	writeOldEpic(t, project, "empty", "", triagedNote)

	by := map[string]oldEpicShown{}
	for _, e := range oldEpics(t, repo) {
		by[e.Epic] = e
	}
	if m := by["movable"]; len(m.Move) != 2 || !m.Archive || m.Waiting {
		t.Errorf("movable = %+v", m)
	}
	if m := by["mixed"]; len(m.Move) != 1 || m.Archive || m.Waiting || len(m.Claimed) != 1 {
		t.Errorf("mixed = %+v", m)
	}
	if m := by["stuck"]; len(m.Move) != 0 || m.Archive || !m.Waiting || len(m.Claimed) != 1 || !strings.HasSuffix(m.Claimed[0], "/10") {
		t.Errorf("stuck = %+v", m)
	}
	for _, name := range []string{"doneonly", "empty"} {
		if m := by[name]; len(m.Move) != 0 || !m.Archive || m.Waiting {
			t.Errorf("%s = %+v", name, m)
		}
	}
	if by["movable"].Backlog != "" {
		t.Errorf("an epic outside the table got backlog %q", by["movable"].Backlog)
	}
}

func TestTriageOldEpics_EmptyListIsAnEmptyArray(t *testing.T) {
	out, err := runIn(t, triageProject(t), "tickets", "triage", "oldepics", "--json")
	if err != nil || strings.TrimSpace(out) != "[]" {
		t.Fatalf("out %q err %v", out, err)
	}
}

func TestOldEpicBacklogs_ShippedTableCoversTheInventory(t *testing.T) {
	table, err := shippedOldEpicBacklogs()
	if err != nil {
		t.Fatal(err)
	}
	backlogs := []string{"test-health", "check-and-brief", "agent-guidance-and-skills", "ticket-authoring",
		"tickets-store-and-cli", "agent-sessions-and-runners", "ralph-loop-and-recovery",
		"epics-queue-and-merge", "server-performance-and-structure", "design-flow", "web-ui", "tui"}
	if got := len(table["gx"]); got != 103 {
		t.Errorf("gx entries = %d, want 103", got)
	}
	for epic, backlog := range table["gx"] {
		if !slices.Contains(backlogs, backlog) {
			t.Errorf("%s -> unknown backlog %q", epic, backlog)
		}
	}
	for epic, want := range map[string]string{
		"commitless-close": "ralph-loop-and-recovery", "fork-handling": "ralph-loop-and-recovery",
		"rtk-hook": "agent-guidance-and-skills", "chat-cards": "web-ui", "chat-notifications": "web-ui",
	} {
		if table["gx"][epic] != want {
			t.Errorf("%s -> %q, want %q", epic, table["gx"][epic], want)
		}
	}
	for _, past := range []string{"agent-workflow-skills", "pause-and-recovery", "pause-cleanup", "queue-and-epics"} {
		if _, ok := table["gx"][past]; ok {
			t.Errorf("past-charting epic %s is in the table", past)
		}
	}
}

func TestTriageOldEpics_ReadsBacklogFromTheTable(t *testing.T) {
	_, project := triageProjectDir(t)
	writeOldEpic(t, project, "chat-cards", "", triagedNote, "open")
	got, err := listOldEpics(project, "gx", map[string]map[string]string{"gx": {"chat-cards": "web-ui"}})
	if err != nil || len(got) != 1 || got[0].Backlog != "web-ui" {
		t.Fatalf("got %+v err %v", got, err)
	}
}
