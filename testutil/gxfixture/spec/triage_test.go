package spec

import (
	"slices"
	"testing"

	"github.com/elentok/gx/tickets/ticketmove"
)

func TestTriageScenario_BacklogSeedsThreeAreasAndAnUnfinishedMap(t *testing.T) {
	t.Parallel()
	sc, ok := TriageScenario("backlog")
	if !ok {
		t.Fatal("no backlog shape")
	}
	open := map[string]int{}
	for _, e := range sc.Epics {
		for _, tk := range e.Tickets {
			if s := tk.Opts.Status; s == "" || s == "open" || s == "draft" {
				open[e.Name]++
			}
		}
	}
	if open["parser"] != 11 || open["sync"] != 5 || open["webui"] != 0 {
		t.Errorf("open tickets = %v, want parser 11, sync 5, webui 0", open)
	}
	area, ids, ok := ticketmove.ParseStartDesignNotes(sc.EpicBody[project+":retry-work"])
	if !ok || area != "sync" || !slices.Contains(ids, "03") {
		t.Errorf("map Notes = area %q ids %v, want sync with a ticket still in the area", area, ids)
	}
}

func TestTriageScenario_OldEpicsSeedsNoAreaInProjectGx(t *testing.T) {
	t.Parallel()
	sc, ok := TriageScenario("old-epics")
	if !ok {
		t.Fatal("no old-epics shape")
	}
	for _, e := range sc.Epics {
		if e.Project != oldEpicsProject {
			t.Errorf("epic %s is in project %s, want %s", e.Name, e.Project, oldEpicsProject)
		}
		if meta := sc.EpicMeta[e.Project+":"+e.Name]; slices.Contains([]string{"status: open\nkind: area\n"}, meta) {
			t.Errorf("epic %s is an area", e.Name)
		}
	}
	if len(sc.Triage) != 1 || len(sc.Steps) != 1 {
		t.Errorf("triage seeds = %d, steps = %d, want one Triage started", len(sc.Triage), len(sc.Steps))
	}
}
