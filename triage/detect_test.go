package triage

import "testing"

func TestNext(t *testing.T) {
	if got := Next(nil); got != BranchStart {
		t.Fatalf("no record = %s", got)
	}

	s, rec := project(t)
	if got := Next(rec); got != BranchVerify {
		t.Fatalf("fresh = %s", got)
	}
	rec, _ = s.SaveVerdict(rec.ID, rec.Revision, Verdict{Address: rowA})
	if got := Next(rec); got != BranchVerify {
		t.Fatalf("one row missing = %s", got)
	}
	rec, _ = s.SaveVerdict(rec.ID, rec.Revision, Verdict{Address: rowB})
	if got := Next(rec); got != BranchPlan {
		t.Fatalf("verdicts in = %s", got)
	}
	rec, _ = s.SavePlan(rec.ID, rec.Revision, []PlanRow{{RowID: rowA, Action: "ready"}}, nil)
	if got := Next(rec); got != BranchPlan {
		t.Fatalf("plan incomplete = %s", got)
	}
	rec, _ = s.SavePlan(rec.ID, rec.Revision, []PlanRow{{RowID: rowB, Action: "ready"}}, nil)
	if got := Next(rec); got != BranchApprove {
		t.Fatalf("plan saved = %s", got)
	}
	rec, _ = s.Approve(rec.ID, rec.Revision)
	if got := Next(rec); got != BranchWrite {
		t.Fatalf("approved = %s", got)
	}
	rec, _ = s.WriteRow(rec.ID, rec.Revision, rowA, WriteDone, "", "")
	rec, _ = s.WriteRow(rec.ID, rec.Revision, rowB, WriteDone, "", "")
	rec, _ = s.SetLastPass(rec.ID, rec.Revision, true)
	if got := Next(rec); got != BranchDone {
		t.Fatalf("done = %s", got)
	}
}

func TestNext_AbandonedStartsOver(t *testing.T) {
	s, rec := project(t)
	rec, _ = s.Abandon(rec.ID, rec.Revision)
	if got := Next(rec); got != BranchStart {
		t.Fatalf("abandoned = %s", got)
	}
}
