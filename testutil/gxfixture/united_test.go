package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
)

// cardsOf is `gx server epics --json`.
func (r *running) cardsOf(t *testing.T) []wire.EpicListRow {
	t.Helper()
	var list wire.EpicList
	if err := json.Unmarshal([]byte(r.gx(t, "server", "epics", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	return list.Epics
}

func TestUnitedScenarioHasACardPerSectionAndReason(t *testing.T) {
	t.Parallel()
	r := start(t, "united")
	// name|section|pill. Running, queued and permission cards appear once the
	// server has launched the workers, so the list is polled.
	want := []string{
		"deadlock-epic|needs-you|Deadlocked",
		"awaiting-epic|needs-you|Awaiting merge",
		"blocked-merge-epic|needs-you|Merge blocked",
		"dirty-trunk-epic|needs-you|Merge blocked",
		"design-stopped|designing|",
		"paused-epic|needs-you|Paused",
		"question-epic|needs-you|Question",
		"perm-epic|needs-you|Permission",
		"design-question|needs-you|Question",
		"paused|needs-you|Paused",
		"running-epic|implementing|",
		"queued-epic|implementing|",
		"queued|implementing|",
		"awaiting-auto-epic|implementing|",
		"design-live|designing|",
		"ready-epic|ready-to-queue|",
		"follow-ups|ready-to-design|",
		"design-new|ready-to-design|",
		"draft-epic|ready-to-design|",
		"idle-epic|ready-to-design|",
		"finished-a|finished|",
	}
	var got []string
	var missing []string
	projects := map[string]bool{}
	for deadline := time.Now().Add(40 * time.Second); time.Now().Before(deadline); time.Sleep(500 * time.Millisecond) {
		got, projects = got[:0], map[string]bool{}
		for _, c := range r.cardsOf(t) {
			got = append(got, c.Name+"|"+string(c.Section)+"|"+c.StatePill)
			projects[c.Project] = true
		}
		missing = slices.DeleteFunc(slices.Clone(want), func(w string) bool { return slices.Contains(got, w) })
		if len(missing) == 0 {
			break
		}
	}
	if len(missing) > 0 {
		t.Fatalf("missing cards %v\ngot:\n%s", missing, strings.Join(got, "\n"))
	}
	if !projects["demo"] || !projects["auto"] {
		t.Errorf("cards span projects %v, want demo and auto", projects)
	}
	// Three finished one-offs exist and server.recently-finished is 2.
	oneOffs := 0
	for _, g := range got {
		if strings.HasPrefix(g, "done-") {
			oneOffs++
		}
	}
	if oneOffs != 2 {
		t.Errorf("%d finished one-offs listed, want 2:\n%s", oneOffs, strings.Join(got, "\n"))
	}
}

// issueFiles is the ticket files of an epic on disk, by file name.
func (r *running) issueFiles(t *testing.T, project, epic string) map[string]string {
	t.Helper()
	root := filepath.Dir(filepath.Dir(r.StateDir))
	dir := filepath.Join(root, "data", "gx", "tickets", project, epic, "issues")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		files[e.Name()] = string(data)
	}
	return files
}

func TestManyDoneScenario_FiveMergedWithDistinctCloseTimesAndOneOpen(t *testing.T) {
	t.Parallel()
	r := start(t, "many-done")
	closed := map[string]bool{}
	open := 0
	for name, data := range r.issueFiles(t, "demo", "many-done-epic") {
		switch {
		case strings.Contains(data, "status: done\n"):
			_, after, ok := strings.Cut(data, "done_at: ")
			if !ok || !strings.Contains(data, "claimed_at: ") {
				t.Fatalf("%s is done without stamps:\n%s", name, data)
			}
			closed[strings.SplitN(after, "\n", 2)[0]] = true
		case strings.Contains(data, "status: open\n"):
			open++
		}
	}
	if len(closed) != 5 || open != 1 {
		t.Errorf("close times %v, open %d; want 5 distinct close times and 1 open", closed, open)
	}
	r.stop(t)
}

func TestUnitedScenario_HasADraftImplEpic(t *testing.T) {
	t.Parallel()
	r := start(t, "united")
	root := filepath.Dir(filepath.Dir(r.StateDir))
	meta, err := os.ReadFile(filepath.Join(root, "data", "gx", "tickets", "demo", "publishing-epic-impl", "ticket.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(meta), "status: draft\n") {
		t.Errorf("publishing-epic-impl ticket.md:\n%s", meta)
	}
	r.stop(t)
}

func TestUnitedScenario_CarriesTheCardTimeStamps(t *testing.T) {
	t.Parallel()
	r := start(t, "united")
	root := filepath.Dir(filepath.Dir(r.StateDir))
	read := func(path ...string) string {
		data, err := os.ReadFile(filepath.Join(append([]string{root, "data", "gx", "tickets"}, path...)...))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	for _, c := range []struct{ project, epic, stamp string }{
		{"demo", "ready-epic", "created_at: "},
		{"demo", "idle-epic", "created_at: "},
		{"demo", "design-live", "design_started_at: "},
		{"demo", "design-stopped", "design_started_at: "},
		{"demo", "running-epic", "started_at: "},
		{"auto", "awaiting-auto-epic", "started_at: "},
		{"demo", "awaiting-epic", "completed_at: "},
		{"demo", "blocked-merge-epic", "completed_at: "},
		{"demo", "dirty-trunk-epic", "completed_at: "},
	} {
		if meta := read(c.project, c.epic, "ticket.md"); !strings.Contains(meta, c.stamp) {
			t.Errorf("%s lacks %q:\n%s", c.epic, c.stamp, meta)
		}
	}
	if got := read("demo", "deadlock-epic", "issues", "01-first.md"); !strings.Contains(got, "claimed_at: ") {
		t.Errorf("deadlock-epic/01 lacks claimed_at:\n%s", got)
	}
	created := strings.SplitN(strings.SplitN(read("demo", "ready-epic", "ticket.md"), "created_at: ", 2)[1], "\n", 2)[0]
	at, err := time.Parse(time.RFC3339, created)
	if err != nil || time.Since(at) > time.Hour {
		t.Errorf("ready-epic created_at %q is not minutes old (%v)", created, err)
	}
	r.stop(t)
}

func TestGrillingStatsScenario_StatsReturnTheSeededTotals(t *testing.T) {
	t.Parallel()
	r := start(t, "grilling-stats")
	tests := []struct {
		args             []string
		words, questions float64
	}{
		{[]string{"--range", "30"}, 550, 12},
		{[]string{"--range", "30", "--project", "demo"}, 310, 7},
		{[]string{"--range", "30", "--project", "other"}, 240, 5},
		{[]string{"--range", "7"}, 260, 6},
		{[]string{"--range", "7", "--project", "demo"}, 60, 2},
		{[]string{"--range", "7", "--project", "other"}, 200, 4},
	}
	for _, tt := range tests {
		var st wire.Stats
		if err := json.Unmarshal([]byte(r.gx(t, append([]string{"server", "stats", "--json"}, tt.args...)...)), &st); err != nil {
			t.Fatal(err)
		}
		var words, questions float64
		for i := range st.Grilling.Words {
			words += st.Grilling.Words[i].Value
			questions += st.Grilling.Questions[i].Value
		}
		if words != tt.words || questions != tt.questions {
			t.Errorf("%v: words %v questions %v, want %v and %v", tt.args, words, questions, tt.words, tt.questions)
		}
	}

	// 30 days: index 0 is 29 days ago. Days 29..26 ago precede the earliest count.
	var st wire.Stats
	if err := json.Unmarshal([]byte(r.gx(t, "server", "stats", "--json", "--range", "30")), &st); err != nil {
		t.Fatal(err)
	}
	for i, w := range st.Grilling.Words {
		daysAgo := 29 - i
		if want := daysAgo > 25; w.NoData != want {
			t.Errorf("%d days ago: no_data = %v, want %v", daysAgo, w.NoData, want)
		}
	}
	if w := st.Grilling.Words[29-24]; w.Value != 0 || w.NoData {
		t.Errorf("24 days ago = %+v, want a zero bar", w)
	}
	r.stop(t)
}
