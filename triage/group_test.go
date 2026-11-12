package triage

import (
	"reflect"
	"testing"
)

// planWith saves a plan for the two fixture rows from the given plan rows.
func planWith(t *testing.T, rows ...PlanRow) *Record {
	t.Helper()
	s, rec := atPlan(t)
	rec, err := s.SavePlan(rec.ID, rec.Revision, rows, nil)
	return must(t, rec, err)
}

func TestSavePlan_SmallOverlappingReadyRowsBecomeOneGroup(t *testing.T) {
	rec := planWith(t,
		PlanRow{RowID: rowA, Action: "ready", Estimate: "10K", OverlapWith: []string{rowB, "other:x/1"}},
		PlanRow{RowID: rowB, Action: "ready", Estimate: "20K", OverlapWith: []string{rowA, "other:x/2"}},
	)
	if rec.Rows[0].Group == "" || rec.Rows[0].Group != rec.Rows[1].Group {
		t.Fatalf("groups = %q, %q, want one shared group", rec.Rows[0].Group, rec.Rows[1].Group)
	}
	if rec.Rows[0].Estimate != "10K" {
		t.Errorf("estimate = %q, want 10K", rec.Rows[0].Estimate)
	}
	if got, want := rec.GroupEdges(rec.Rows[0].Group), []string{"other:x/1", "other:x/2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("group edges = %v, want the union of the members' %v", got, want)
	}
}

func TestSavePlan_ChainsInsteadOfGrouping(t *testing.T) {
	for name, rows := range map[string][]PlanRow{
		"a member has no estimate": {
			{RowID: rowA, Action: "ready", Estimate: "10K", OverlapWith: []string{rowB}},
			{RowID: rowB, Action: "ready", OverlapWith: []string{rowA}},
		},
		"combined estimate is over the limit": {
			{RowID: rowA, Action: "ready", Estimate: "30K", OverlapWith: []string{rowB}},
			{RowID: rowB, Action: "ready", Estimate: "15K", OverlapWith: []string{rowA}},
		},
		"a map row": {
			{RowID: rowA, Action: "map", Destination: "m", Estimate: "5K", OverlapWith: []string{rowB}},
			{RowID: rowB, Action: "ready", Estimate: "5K", OverlapWith: []string{rowA}},
		},
		"no overlap": {
			{RowID: rowA, Action: "ready", Estimate: "5K"},
			{RowID: rowB, Action: "ready", Estimate: "5K"},
		},
	} {
		rec := planWith(t, rows...)
		if rec.Rows[0].Group != "" || rec.Rows[1].Group != "" {
			t.Errorf("%s: grouped %q, %q", name, rec.Rows[0].Group, rec.Rows[1].Group)
		}
	}
}

func TestUngroup_RestoresTheMembersWithTheirOverlapChain(t *testing.T) {
	s, _ := atPlan(t)
	rec, _ := s.LoadLive()
	rec, err := s.SavePlan(rec.ID, rec.Revision, []PlanRow{
		{RowID: rowA, Action: "ready", Estimate: "10K", OverlapWith: []string{rowB}},
		{RowID: rowB, Action: "ready", Estimate: "10K", OverlapWith: []string{rowA}},
	}, nil)
	must(t, rec, err)
	rec, err = s.Ungroup(rec.ID, rec.Revision, rowB)
	must(t, rec, err)
	for _, r := range rec.Rows {
		if r.Group != "" || len(r.Plan.OverlapWith) != 1 {
			t.Errorf("row %s group %q overlap %v, want ungrouped and chained", r.Address, r.Group, r.Plan.OverlapWith)
		}
	}
	if _, err := s.Ungroup(rec.ID, rec.Revision, rowA); err == nil {
		t.Error("ungroup of an ungrouped row accepted")
	}
}

func TestUngroup_OnlyAtApprove(t *testing.T) {
	s, rec := atPlan(t)
	if _, err := s.Ungroup(rec.ID, rec.Revision, rowA); err == nil {
		t.Fatal("ungroup accepted at step plan")
	}
}
