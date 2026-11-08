package triage

import (
	"errors"
	"fmt"
	"slices"

	"github.com/elentok/gx/tickets"
)

// Verdicts a Verify session may store.
const (
	VerdictRelevant = "relevant"
	VerdictFixed    = "fixed"
	VerdictObsolete = "obsolete"
)

// Plan actions.
var planActions = []string{"ready", "map", "merge", "close", "keep"}

// ErrAlreadyApproved is returned by Approve when the step is past approve.
var ErrAlreadyApproved = errors.New("already approved")

// PhaseError is returned when an operation is not allowed at the stored step
// or in the row's state.
type PhaseError struct{ Msg string }

func (e *PhaseError) Error() string { return e.Msg }

func phaseErr(format string, args ...any) error {
	return &PhaseError{Msg: fmt.Sprintf(format, args...)}
}

// Verdict is what a Verify session saves for one row.
type Verdict struct {
	Address              string
	Verdict              string // empty counts as relevant
	Evidence             string
	Files                []string
	Questions            []Question
	SuggestedDestination string
	// Gone marks a follow-up already closed or moved out by someone else.
	Gone bool
}

// modify runs fn on the record under the folder lock and saves the result.
// fn runs before the revision check, so a phase refusal ("already approved")
// wins over a stale one. rec is returned with its new revision.
func (s *Store) modify(id string, revision int, fn func(*Record) error) (*Record, error) {
	unlock, err := s.lock()
	if err != nil {
		return nil, err
	}
	defer unlock()

	rec, err := s.Load(id)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, fmt.Errorf("triage %s not found", id)
	}
	if rec.State != StateLive {
		return nil, phaseErr("triage %s is %s", id, rec.State)
	}
	if err := fn(rec); err != nil {
		return nil, err
	}
	if rec.Revision != revision {
		return nil, ErrStaleRevision
	}
	finishIfDone(rec)
	if err := s.saveLocked(rec); err != nil {
		return nil, err
	}
	return rec, nil
}

func (r *Record) row(address string) (*Row, error) {
	for i := range r.Rows {
		if r.Rows[i].Address == address {
			return &r.Rows[i], nil
		}
	}
	return nil, fmt.Errorf("no row %q in triage %s", address, r.ID)
}

func (r *Row) gone() bool { return r.Write == WriteGone }

func requireStep(rec *Record, what string, steps ...string) error {
	if !slices.Contains(steps, rec.Step) {
		return phaseErr("%s is not allowed at step %s", what, rec.Step)
	}
	return nil
}

// PendingVerdicts lists the rows with no verdict that are not gone.
func (r *Record) PendingVerdicts() []Row {
	var out []Row
	for _, row := range r.Rows {
		if row.Verdict == "" && !row.gone() {
			out = append(out, row)
		}
	}
	return out
}

// planPending reports whether a row that is not gone still has no plan.
func (r *Record) planPending() bool {
	for _, row := range r.Rows {
		if !row.gone() && row.Plan == nil {
			return true
		}
	}
	return false
}

// SaveVerdict stores one row's verdict during Verify. When the last row has a
// verdict (or is gone) the step moves to plan.
func (s *Store) SaveVerdict(id string, revision int, v Verdict) (*Record, error) {
	return s.modify(id, revision, func(rec *Record) error {
		if err := requireStep(rec, "verdict save", tickets.StepVerify); err != nil {
			return err
		}
		row, err := rec.row(v.Address)
		if err != nil {
			return err
		}
		if v.Gone {
			row.Write = WriteGone
		} else {
			switch v.Verdict {
			case "":
				v.Verdict = VerdictRelevant
			case VerdictRelevant, VerdictFixed, VerdictObsolete:
			default:
				return phaseErr("unknown verdict %q", v.Verdict)
			}
			row.Verdict = v.Verdict
			row.Evidence = v.Evidence
			row.Files = v.Files
			row.Questions = v.Questions
			row.SuggestedDestination = v.SuggestedDestination
		}
		if len(rec.PendingVerdicts()) == 0 {
			rec.Step = tickets.StepPlan
		}
		return nil
	})
}

// SavePlan stores plan rows during Plan, and marks rows found gone. Plan rows
// for gone rows are left out. When every row that is not gone has a plan the
// step moves to approve.
func (s *Store) SavePlan(id string, revision int, plan []PlanRow, gone []string) (*Record, error) {
	return s.modify(id, revision, func(rec *Record) error {
		if err := requireStep(rec, "plan save", tickets.StepPlan); err != nil {
			return err
		}
		for _, addr := range gone {
			row, err := rec.row(addr)
			if err != nil {
				return err
			}
			row.Write = WriteGone
			row.Plan = nil
		}
		for _, p := range plan {
			row, err := rec.row(p.RowID)
			if err != nil {
				return err
			}
			if !slices.Contains(planActions, p.Action) {
				return phaseErr("unknown plan action %q", p.Action)
			}
			if row.gone() {
				continue
			}
			p := p
			row.Plan = &p
		}
		if !rec.planPending() {
			rec.Step = tickets.StepApprove
		}
		return nil
	})
}

// SetPerson replaces a row's person fields. Allowed only at step approve.
func (s *Store) SetPerson(id string, revision int, address string, f PersonFields) (*Record, error) {
	return s.modify(id, revision, func(rec *Record) error {
		if err := requireStep(rec, "person fields", tickets.StepApprove); err != nil {
			return err
		}
		row, err := rec.row(address)
		if err != nil {
			return err
		}
		if row.gone() {
			return phaseErr("row %s is gone", address)
		}
		if f.Action != "" && !slices.Contains(planActions, f.Action) {
			return phaseErr("unknown action %q", f.Action)
		}
		row.Person = f
		return nil
	})
}

// Approve moves the step from approve to write: a compare-and-set on the
// revision. A step past approve is "already approved"; an unchanged step with
// a changed revision is stale.
func (s *Store) Approve(id string, revision int) (*Record, error) {
	return s.modify(id, revision, func(rec *Record) error {
		switch rec.Step {
		case tickets.StepApprove:
			rec.Step = tickets.StepWrite
			return nil
		case tickets.StepVerify, tickets.StepPlan:
			return phaseErr("approve is not allowed at step %s", rec.Step)
		}
		return ErrAlreadyApproved
	})
}

// WriteRow stores per-row write state during Write. A successful state clears
// the refusal text.
func (s *Store) WriteRow(id string, revision int, address string, state WriteState, newAddress, refusal string) (*Record, error) {
	return s.modify(id, revision, func(rec *Record) error {
		if err := requireStep(rec, "write state", tickets.StepWrite); err != nil {
			return err
		}
		row, err := rec.row(address)
		if err != nil {
			return err
		}
		switch state {
		case WritePending, WriteDone, WriteFailed, WriteGone:
		default:
			return phaseErr("unknown write state %q", state)
		}
		row.Write = state
		if newAddress != "" {
			row.NewAddress = newAddress
		}
		row.Refusal = ""
		if state == WriteFailed {
			row.Refusal = refusal
		}
		return nil
	})
}

// SetLastPass stores the last-pass-complete flag during Write.
func (s *Store) SetLastPass(id string, revision int, complete bool) (*Record, error) {
	return s.modify(id, revision, func(rec *Record) error {
		if err := requireStep(rec, "last-pass flag", tickets.StepWrite); err != nil {
			return err
		}
		rec.LastPassComplete = complete
		return nil
	})
}

// Retry turns a failed row back to pending. Allowed only at step write.
func (s *Store) Retry(id string, revision int, address string) (*Record, error) {
	return s.modify(id, revision, func(rec *Record) error {
		if err := requireStep(rec, "retry", tickets.StepWrite); err != nil {
			return err
		}
		row, err := rec.row(address)
		if err != nil {
			return err
		}
		if row.Write != WriteFailed {
			return phaseErr("row %s is %s, not failed", address, row.Write)
		}
		row.Write = WritePending
		return nil
	})
}

// KeepForLater keeps a failed row in follow-ups. Allowed only at step write
// and only while the row has no new address, so a half-moved ticket is never
// called kept.
func (s *Store) KeepForLater(id string, revision int, address string) (*Record, error) {
	return s.modify(id, revision, func(rec *Record) error {
		if err := requireStep(rec, "keep for later", tickets.StepWrite); err != nil {
			return err
		}
		row, err := rec.row(address)
		if err != nil {
			return err
		}
		if row.Write != WriteFailed {
			return phaseErr("row %s is %s, not failed", address, row.Write)
		}
		if row.NewAddress != "" {
			return phaseErr("row %s already moved to %s", address, row.NewAddress)
		}
		row.Person.KeepForLater = true
		return nil
	})
}

// Abandon ends a live Triage that is not yet at write. It writes nothing to
// tickets.
func (s *Store) Abandon(id string, revision int) (*Record, error) {
	return s.modify(id, revision, func(rec *Record) error {
		if err := requireStep(rec, "abandon", tickets.StepVerify, tickets.StepPlan, tickets.StepApprove); err != nil {
			return err
		}
		rec.State = StateAbandoned
		return nil
	})
}

// finishIfDone makes a live record done when every row is done, kept or gone
// and the last pass is complete.
func finishIfDone(rec *Record) {
	if rec.State != StateLive || rec.Step != tickets.StepWrite || !rec.LastPassComplete {
		return
	}
	for _, row := range rec.Rows {
		if row.Write != WriteDone && !row.Person.KeepForLater && !row.gone() {
			return
		}
	}
	rec.State = StateDone
	rec.Step = string(tickets.StepDone)
}
