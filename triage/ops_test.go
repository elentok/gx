package triage

import (
	"errors"
	"sync"
	"testing"
)

const (
	rowA = "proj:follow-ups/01"
	rowB = "proj:follow-ups/02"
)

// project builds a Triage with two rows. Row addresses are set explicitly so
// tests do not depend on the snapshot's address format.
func project(t *testing.T) (*Store, *Record) {
	t.Helper()
	s, _ := newProject(t)
	rec, err := s.Create()
	if err != nil {
		t.Fatal(err)
	}
	rec.Rows[0].Address, rec.Rows[1].Address = rowA, rowB
	cur, err := s.Load(rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	cur.Rows = rec.Rows
	if err := s.Save(cur); err != nil {
		t.Fatal(err)
	}
	return s, cur
}

func must(t *testing.T, rec *Record, err error) *Record {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func atPlan(t *testing.T) (*Store, *Record) {
	s, rec := project(t)
	rec = must(t, rec, nil)
	for _, a := range []string{rowA, rowB} {
		var err error
		rec, err = s.SaveVerdict(rec.ID, rec.Revision, Verdict{Address: a, Verdict: VerdictRelevant})
		must(t, rec, err)
	}
	return s, rec
}

func atApprove(t *testing.T) (*Store, *Record) {
	s, rec := atPlan(t)
	rec, err := s.SavePlan(rec.ID, rec.Revision, []PlanRow{{RowID: rowA, Action: "ready"}, {RowID: rowB, Action: "close"}}, nil)
	return s, must(t, rec, err)
}

func atWrite(t *testing.T) (*Store, *Record) {
	s, rec := atApprove(t)
	rec, err := s.Approve(rec.ID, rec.Revision)
	return s, must(t, rec, err)
}

func TestVerdict(t *testing.T) {
	s, rec := project(t)
	rec, err := s.SaveVerdict(rec.ID, rec.Revision, Verdict{Address: rowA, Evidence: "e", Files: []string{"a.go"}, SuggestedDestination: "map-x"})
	must(t, rec, err)
	if rec.Rows[0].Verdict != VerdictRelevant || rec.Rows[0].Evidence != "e" || rec.Step != "verify" {
		t.Fatalf("undecided should count as relevant and keep verify: %+v", rec.Rows[0])
	}
	if p := rec.PendingVerdicts(); len(p) != 1 || p[0].Address != rowB {
		t.Fatalf("pending = %+v", p)
	}
	// a fresh session reads the stored record and sees only what is missing
	fresh, _ := s.LoadLive()
	if p := fresh.PendingVerdicts(); len(p) != 1 || p[0].Address != rowB {
		t.Fatalf("fresh pending = %+v", p)
	}
	if _, err := s.SaveVerdict(rec.ID, rec.Revision, Verdict{Address: rowB, Verdict: "bogus"}); err == nil {
		t.Fatal("unknown verdict accepted")
	}
	if _, err := s.SaveVerdict(rec.ID, rec.Revision, Verdict{Address: "nope", Verdict: VerdictFixed}); err == nil {
		t.Fatal("unknown row accepted")
	}
	rec, err = s.SaveVerdict(rec.ID, rec.Revision, Verdict{Address: rowB, Gone: true})
	must(t, rec, err)
	if rec.Step != "plan" || rec.Rows[1].Write != WriteGone {
		t.Fatalf("last row (gone) should move the step to plan: %+v", rec)
	}
	if _, err := s.SaveVerdict(rec.ID, rec.Revision, Verdict{Address: rowA, Verdict: VerdictFixed}); err == nil {
		t.Fatal("verdict accepted after verify")
	}
}

func TestSecondSaveOnOneRevisionRefused(t *testing.T) {
	s, rec := project(t)
	rev := rec.Revision
	if _, err := s.SaveVerdict(rec.ID, rev, Verdict{Address: rowA}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveVerdict(rec.ID, rev, Verdict{Address: rowB}); !errors.Is(err, ErrStaleRevision) {
		t.Fatalf("err = %v, want stale", err)
	}
}

func TestPlan(t *testing.T) {
	s, rec := atPlan(t)
	if _, err := s.SavePlan(rec.ID, rec.Revision, []PlanRow{{RowID: rowA, Action: "bogus"}}, nil); err == nil {
		t.Fatal("unknown action accepted")
	}
	rec, err := s.SavePlan(rec.ID, rec.Revision, []PlanRow{{RowID: rowA, Action: "map", Destination: "m"}}, nil)
	must(t, rec, err)
	if rec.Step != "plan" || rec.Rows[0].Plan == nil || rec.Rows[0].Plan.Destination != "m" {
		t.Fatalf("partial plan: %+v", rec)
	}
	// B goes gone during Plan; a plan row naming it is left out
	rec, err = s.SavePlan(rec.ID, rec.Revision, []PlanRow{{RowID: rowB, Action: "ready"}}, []string{rowB})
	must(t, rec, err)
	if rec.Rows[1].Plan != nil || rec.Rows[1].Write != WriteGone || rec.Step != "approve" {
		t.Fatalf("gone row should be left out and last save move to approve: %+v", rec)
	}
	if _, err := s.SavePlan(rec.ID, rec.Revision, nil, nil); err == nil {
		t.Fatal("plan save accepted after plan")
	}
}

func TestPersonFields(t *testing.T) {
	s, rec := atPlan(t)
	if _, err := s.SetPerson(rec.ID, rec.Revision, rowA, PersonFields{Action: "keep"}); err == nil {
		t.Fatal("person fields accepted before approve")
	}
	rec, _ = s.SavePlan(rec.ID, rec.Revision, []PlanRow{{RowID: rowA, Action: "ready"}, {RowID: rowB, Action: "ready"}}, nil)
	if _, err := s.SetPerson(rec.ID, rec.Revision, rowA, PersonFields{Action: "bogus"}); err == nil {
		t.Fatal("unknown action accepted")
	}
	rec, err := s.SetPerson(rec.ID, rec.Revision, rowA, PersonFields{Action: "map", Destination: "d", Answers: map[string]string{"q": "a"}})
	must(t, rec, err)
	if rec.Rows[0].Person.Destination != "d" || rec.Rows[0].Person.Answers["q"] != "a" {
		t.Fatalf("person = %+v", rec.Rows[0].Person)
	}
	rec, _ = s.Approve(rec.ID, rec.Revision)
	if _, err := s.SetPerson(rec.ID, rec.Revision, rowA, PersonFields{Action: "keep"}); err == nil {
		t.Fatal("person fields accepted after approve")
	}
}

func TestApprove(t *testing.T) {
	s, rec := atApprove(t)
	rev := rec.Revision

	// a person-field save in between makes the approve stale
	if _, err := s.SetPerson(rec.ID, rev, rowA, PersonFields{Action: "keep"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Approve(rec.ID, rev); !errors.Is(err, ErrStaleRevision) {
		t.Fatalf("err = %v, want stale", err)
	}
	cur, _ := s.LoadLive()
	won, err := s.Approve(cur.ID, cur.Revision)
	if err != nil || won.Step != "write" {
		t.Fatalf("approve = %+v, %v", won, err)
	}
	if _, err := s.Approve(cur.ID, cur.Revision); !errors.Is(err, ErrAlreadyApproved) {
		t.Fatalf("err = %v, want already approved", err)
	}
}

func TestApprove_BeforeApproveStepRefused(t *testing.T) {
	s, rec := atPlan(t)
	if _, err := s.Approve(rec.ID, rec.Revision); err == nil || errors.Is(err, ErrAlreadyApproved) {
		t.Fatalf("err = %v", err)
	}
}

func TestApprove_RaceFirstWinsSecondAlreadyApproved(t *testing.T) {
	s, rec := atApprove(t)
	const n = 8
	var wg sync.WaitGroup
	var mu sync.Mutex
	won, already, stale := 0, 0, 0
	for range n {
		wg.Go(func() {
			_, err := s.Approve(rec.ID, rec.Revision)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				won++
			case errors.Is(err, ErrAlreadyApproved):
				already++
			case errors.Is(err, ErrStaleRevision):
				stale++
			default:
				t.Errorf("err = %v", err)
			}
		})
	}
	wg.Wait()
	if won != 1 || already != n-1 || stale != 0 {
		t.Fatalf("won=%d already=%d stale=%d", won, already, stale)
	}
}

func TestWriteState(t *testing.T) {
	s, rec := atApprove(t)
	if _, err := s.WriteRow(rec.ID, rec.Revision, rowA, WriteDone, "", ""); err == nil {
		t.Fatal("write state accepted before write")
	}
	rec, _ = s.Approve(rec.ID, rec.Revision)
	rec, err := s.WriteRow(rec.ID, rec.Revision, rowA, WriteFailed, "", "server-not-running")
	must(t, rec, err)
	if rec.Rows[0].Refusal != "server-not-running" {
		t.Fatalf("row = %+v", rec.Rows[0])
	}
	rec, err = s.WriteRow(rec.ID, rec.Revision, rowA, WriteDone, "proj:map/01", "")
	must(t, rec, err)
	if rec.Rows[0].Refusal != "" || rec.Rows[0].NewAddress != "proj:map/01" {
		t.Fatalf("a successful rerun should clear the refusal: %+v", rec.Rows[0])
	}
	if _, err := s.WriteRow(rec.ID, rec.Revision, rowA, "bogus", "", ""); err == nil {
		t.Fatal("unknown state accepted")
	}
}

func TestRetryAndKeepForLater(t *testing.T) {
	s, rec := atApprove(t)
	if _, err := s.Retry(rec.ID, rec.Revision, rowA); err == nil {
		t.Fatal("retry accepted before write")
	}
	if _, err := s.KeepForLater(rec.ID, rec.Revision, rowA); err == nil {
		t.Fatal("keep accepted before write")
	}
	rec, _ = s.Approve(rec.ID, rec.Revision)
	if _, err := s.Retry(rec.ID, rec.Revision, rowA); err == nil {
		t.Fatal("retry of a pending row accepted")
	}
	rec, _ = s.WriteRow(rec.ID, rec.Revision, rowA, WriteFailed, "", "boom")
	rec, err := s.Retry(rec.ID, rec.Revision, rowA)
	must(t, rec, err)
	if rec.Rows[0].Write != WritePending {
		t.Fatalf("row = %+v", rec.Rows[0])
	}

	// failed after a move: Keep for later refused
	rec, _ = s.WriteRow(rec.ID, rec.Revision, rowA, WriteFailed, "proj:map/01", "boom")
	if _, err := s.KeepForLater(rec.ID, rec.Revision, rowA); err == nil {
		t.Fatal("keep accepted after a move")
	}
	// failed in follow-ups: accepted
	rec, _ = s.WriteRow(rec.ID, rec.Revision, rowB, WriteFailed, "", "boom")
	rec, err = s.KeepForLater(rec.ID, rec.Revision, rowB)
	must(t, rec, err)
	if !rec.Rows[1].Person.KeepForLater || rec.Rows[1].Write != WriteFailed {
		t.Fatalf("keep clears nothing: %+v", rec.Rows[1])
	}
}

func TestDoneRule(t *testing.T) {
	t.Run("gone during Write", func(t *testing.T) {
		s, rec := atWrite(t)
		rec, _ = s.WriteRow(rec.ID, rec.Revision, rowA, WriteDone, "", "")
		rec, _ = s.WriteRow(rec.ID, rec.Revision, rowB, WriteGone, "", "")
		if rec.State != StateLive {
			t.Fatal("done before the last pass")
		}
		rec, err := s.SetLastPass(rec.ID, rec.Revision, true)
		must(t, rec, err)
		assertDone(t, s, rec)
	})
	t.Run("gone during Verify, kept row", func(t *testing.T) {
		s, rec := project(t)
		rec, _ = s.SaveVerdict(rec.ID, rec.Revision, Verdict{Address: rowA, Gone: true})
		rec, _ = s.SaveVerdict(rec.ID, rec.Revision, Verdict{Address: rowB})
		rec, _ = s.SavePlan(rec.ID, rec.Revision, []PlanRow{{RowID: rowB, Action: "ready"}}, nil)
		rec, _ = s.Approve(rec.ID, rec.Revision)
		rec, _ = s.WriteRow(rec.ID, rec.Revision, rowB, WriteFailed, "", "boom")
		rec, _ = s.SetLastPass(rec.ID, rec.Revision, true)
		if rec.State != StateLive {
			t.Fatal("failed row should not be done")
		}
		rec, err := s.KeepForLater(rec.ID, rec.Revision, rowB)
		must(t, rec, err)
		assertDone(t, s, rec)
	})
	t.Run("flag unset", func(t *testing.T) {
		s, rec := atWrite(t)
		rec, _ = s.WriteRow(rec.ID, rec.Revision, rowA, WriteDone, "", "")
		rec, _ = s.WriteRow(rec.ID, rec.Revision, rowB, WriteDone, "", "")
		if rec.State != StateLive || rec.Step != "write" {
			t.Fatalf("crash before the last pass must leave it not done: %+v", rec)
		}
		if live, _ := s.LoadLive(); live == nil {
			t.Fatal("marker released early")
		}
	})
}

func assertDone(t *testing.T, s *Store, rec *Record) {
	t.Helper()
	if rec.State != StateDone || rec.Step != "done" {
		t.Fatalf("not done: %+v", rec)
	}
	if live, _ := s.LoadLive(); live != nil {
		t.Fatal("marker not released")
	}
}

func TestAbandon(t *testing.T) {
	cases := []struct {
		name string
		to   func(*testing.T) (*Store, *Record)
		ok   bool
	}{
		{"verify", project, true},
		{"plan", atPlan, true},
		{"approve", atApprove, true},
		{"write", atWrite, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, rec := c.to(t)
			got, err := s.Abandon(rec.ID, rec.Revision)
			if (err == nil) != c.ok {
				t.Fatalf("err = %v, want ok=%v", err, c.ok)
			}
			if !c.ok {
				return
			}
			if got.State != StateAbandoned {
				t.Fatalf("state = %s", got.State)
			}
			if live, _ := s.LoadLive(); live != nil {
				t.Fatal("marker not released")
			}
			if h, _ := s.History(); len(h) != 1 {
				t.Fatalf("history = %d", len(h))
			}
		})
	}
}
