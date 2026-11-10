package prflow_test

import (
	"slices"
	"testing"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/ghfake"
)

func epicRow(t *testing.T, list wire.EpicList) wire.EpicListRow {
	t.Helper()
	i := slices.IndexFunc(list.Epics, func(r wire.EpicListRow) bool { return r.Address == "proj:epic-a" })
	if i < 0 {
		t.Fatalf("no row for proj:epic-a in %+v", list.Epics)
	}
	return list.Epics[i]
}

func TestLiveWire_ChipSurvivesARestartThenFollowsThePR(t *testing.T) {
	f := openRig(t, "")
	setProject(t, f, `,"delivery":"pr"`)
	openPR(f)
	f.StampPR(t, 5, prURL)

	// Before the first poll the chip comes from the epic's own fields.
	list, err := f.H.Client.Reads.Epics(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if row := epicRow(t, list); row.PR == nil || row.PR.Chip != "#5" || row.PR.URL != prURL || row.StatusWords != "PR open" {
		t.Fatalf("before the first poll: pr %+v words %q", row.PR, row.StatusWords)
	}

	f.Tick(t)
	list, err = f.H.Client.Reads.Epics(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	row := epicRow(t, list)
	if row.PR == nil || row.PR.Chip != "#5 · draft" || row.PR.Checks != "1 of 1 checks running" || row.StatusWords != "PR draft · CI running" {
		t.Fatalf("draft running: pr %+v words %q", row.PR, row.StatusWords)
	}
	if !slices.ContainsFunc(row.Actions, func(a wire.CardAction) bool {
		return a.Kind == wire.CardActionOpenPR && a.Link == prURL
	}) || slices.ContainsFunc(row.Actions, func(a wire.CardAction) bool { return a.Kind == wire.CardActionCreatePR }) {
		t.Errorf("actions = %+v, want Open on GitHub and no Create PR", row.Actions)
	}

	// Green and ready: the words change and the detail lists the activity.
	f.GH.Update(func(s *ghfake.State) {
		s.PRs[0].Draft = false
		s.Checks["sha1"][0] = ghfake.Check{Name: "build", Status: "COMPLETED", Conclusion: "SUCCESS", JobID: 1, RunID: 1, WorkflowID: 1}
	})
	f.Tick(t)
	d, err := f.H.Client.Reads.EpicDetail(t.Context(), "proj:epic-a")
	if err != nil {
		t.Fatal(err)
	}
	if d.PR == nil || d.PR.Chip != "#5 · ready" || d.StatusWords != "Ready for review" || d.PR.Checks != "1 of 1 checks passed" {
		t.Errorf("ready: pr %+v words %q", d.PR, d.StatusWords)
	}
}

func TestLiveWire_MergedPRCarriesItsActivityNewestFirst(t *testing.T) {
	f, _ := mergedRig(t)
	f.Tick(t)
	d, err := f.H.Client.Reads.EpicDetail(t.Context(), "proj:epic-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.PRActivity) == 0 {
		t.Fatal("no PR activity")
	}
	for i := 1; i < len(d.PRActivity); i++ {
		if d.PRActivity[i].Time.After(d.PRActivity[i-1].Time) {
			t.Errorf("activity not newest first: %+v", d.PRActivity)
		}
	}
	if d.PR == nil || d.PR.State != wire.PRMerged {
		t.Errorf("pr = %+v, want merged", d.PR)
	}
}
