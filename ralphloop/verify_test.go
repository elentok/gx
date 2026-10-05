package ralphloop

import (
	"errors"
	"testing"

	"github.com/elentok/gx/herdr"
	"github.com/elentok/gx/tickets"
)

// verifyFixture returns a VerifyDeps where nothing has landed and nothing is
// left behind; each test overrides only the rung it exercises.
func verifyFixture() VerifyDeps {
	return VerifyDeps{
		IsAncestor:     func(dir, ancestor, descendant string) (bool, error) { return false, nil },
		MergeBase:      func(dir, refA, refB string) (string, error) { return "base", nil },
		PatchesApplied: func(dir, upstream, base, branch string) (bool, error) { return false, nil },
		RevParse:       func(dir, ref string) (string, error) { return "", errors.New("unknown revision") },
		WorktreeExists: func(path string) (bool, error) { return false, nil },
		TabList:        func(string) ([]herdr.Tab, error) { return nil, nil },
		LandedTickets:  func(dir, branch string) (map[string]bool, error) { return map[string]bool{}, nil },
	}
}

func verifyOne(t *testing.T, vd VerifyDeps, tk tickets.Ticket, events []Event) TicketVerification {
	t.Helper()
	got, err := VerifyEpic(vd, VerifyParams{Epic: "epic", Tickets: []tickets.Ticket{tk}, Events: events})
	if err != nil {
		t.Fatalf("VerifyEpic() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	return got[0]
}

func doneTicket() tickets.Ticket {
	return tickets.Ticket{Number: 3, Identifier: "03", Status: "done"}
}

func pickedEvent() []Event {
	return []Event{{Type: eventCherryPicked, Ticket: "03", SHA: "abc123"}}
}

func TestVerifyEpic_SHARung(t *testing.T) {
	t.Parallel()
	vd := verifyFixture()
	vd.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return true, nil }
	v := verifyOne(t, vd, doneTicket(), pickedEvent())
	if v.Landing != LandingLanded || v.Evidence != EvidenceSHA || v.SHA != "abc123" {
		t.Errorf("got %+v, want landed via sha abc123", v)
	}
}

func TestVerifyEpic_PatchIDRung(t *testing.T) {
	t.Parallel()
	vd := verifyFixture()
	vd.RevParse = func(dir, ref string) (string, error) { return "deadbeef", nil }
	vd.PatchesApplied = func(dir, upstream, base, branch string) (bool, error) { return true, nil }
	v := verifyOne(t, vd, doneTicket(), pickedEvent())
	if v.Landing != LandingLanded || v.Evidence != EvidencePatchID {
		t.Errorf("got %+v, want landed via patch-id", v)
	}
	if v.Leftovers.Branch == nil || !*v.Leftovers.Branch {
		t.Errorf("Leftovers.Branch = %v, want true", v.Leftovers.Branch)
	}
}

func TestVerifyEpic_TrailerRung(t *testing.T) {
	t.Parallel()
	vd := verifyFixture()
	vd.LandedTickets = func(dir, branch string) (map[string]bool, error) { return map[string]bool{"03": true}, nil }
	v := verifyOne(t, vd, doneTicket(), nil)
	if v.Landing != LandingLanded || v.Evidence != EvidenceTrailer {
		t.Errorf("got %+v, want landed via trailer", v)
	}
}

func TestVerifyEpic_NothingLanded(t *testing.T) {
	t.Parallel()
	vd := verifyFixture()
	v := verifyOne(t, vd, doneTicket(), pickedEvent())
	if v.Landing != LandingUnrecoverable || v.Evidence != EvidenceNone {
		t.Errorf("got %+v, want unrecoverable with no evidence", v)
	}

	vd.RevParse = func(dir, ref string) (string, error) { return "deadbeef", nil }
	v = verifyOne(t, vd, doneTicket(), pickedEvent())
	if v.Landing != LandingRecoverable {
		t.Errorf("Landing = %q, want recoverable while the iteration branch survives", v.Landing)
	}
}

func TestVerifyEpic_UnknownWhenCheckCannotRun(t *testing.T) {
	t.Parallel()
	vd := verifyFixture()
	vd.LandedTickets = func(dir, branch string) (map[string]bool, error) { return nil, errors.New("not a git repo") }
	v := verifyOne(t, vd, doneTicket(), nil)
	if v.Landing != LandingUnknown || !v.Unknown {
		t.Errorf("got %+v, want unknown when the trailer rung cannot run", v)
	}

	vd = verifyFixture()
	vd.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return false, errors.New("boom") }
	v = verifyOne(t, vd, doneTicket(), pickedEvent())
	if v.Landing != LandingUnknown || !v.Unknown {
		t.Errorf("got %+v, want unknown when IsAncestor fails", v)
	}
}

func TestVerifyEpic_EarlierRungAnswersDespiteTrailerFailure(t *testing.T) {
	t.Parallel()
	vd := verifyFixture()
	vd.IsAncestor = func(dir, ancestor, descendant string) (bool, error) { return true, nil }
	vd.LandedTickets = func(dir, branch string) (map[string]bool, error) { return nil, errors.New("not a git repo") }
	v := verifyOne(t, vd, doneTicket(), pickedEvent())
	if v.Landing != LandingLanded || v.Unknown {
		t.Errorf("got %+v, want landed and not unknown", v)
	}
}

func TestVerifyEpic_ClaimedTicketWithLeftoversByDesign(t *testing.T) {
	t.Parallel()
	vd := verifyFixture()
	vd.RevParse = func(dir, ref string) (string, error) { return "deadbeef", nil }
	vd.WorktreeExists = func(path string) (bool, error) { return true, nil }
	vd.TabList = func(string) ([]herdr.Tab, error) { return []herdr.Tab{{Label: iterLabel("epic", "03")}}, nil }
	tk := tickets.Ticket{Number: 3, Identifier: "03", Status: "claimed"}
	v := verifyOne(t, vd, tk, nil)
	l := v.Leftovers
	if l.Tab == nil || !*l.Tab || l.Worktree == nil || !*l.Worktree || l.Branch == nil || !*l.Branch {
		t.Errorf("Leftovers = %+v, want tab, worktree and branch all true", l)
	}
	if v.Landing == LandingLanded {
		t.Errorf("Landing = %q, a claimed ticket with nothing landed must not read landed", v.Landing)
	}
}

func TestVerifyEpic_NotExpected(t *testing.T) {
	t.Parallel()
	vd := verifyFixture()
	commitless := tickets.Ticket{Number: 3, Identifier: "03", Status: "done", Commitless: true}
	if v := verifyOne(t, vd, commitless, nil); v.Landing != LandingNotExpected {
		t.Errorf("commitless Landing = %q, want not-expected", v.Landing)
	}
	open := tickets.Ticket{Number: 3, Identifier: "03", Status: "open"}
	if v := verifyOne(t, vd, open, nil); v.Landing != LandingNotExpected {
		t.Errorf("never-iterated Landing = %q, want not-expected", v.Landing)
	}
}

func TestVerifyEpic_NilTabListMeansTabUnanswered(t *testing.T) {
	t.Parallel()
	vd := verifyFixture()
	vd.TabList = nil
	v := verifyOne(t, vd, doneTicket(), nil)
	if v.Leftovers.Tab != nil {
		t.Errorf("Leftovers.Tab = %v, want nil when herdr did not answer", *v.Leftovers.Tab)
	}
	if v.Leftovers.Worktree == nil || v.Leftovers.Branch == nil {
		t.Errorf("Leftovers = %+v, git-side fields must still be answered", v.Leftovers)
	}

	vd.TabList = func(string) ([]herdr.Tab, error) { return nil, errors.New("herdr down") }
	if v := verifyOne(t, vd, doneTicket(), nil); v.Leftovers.Tab != nil {
		t.Errorf("Leftovers.Tab = %v, want nil when TabList errors", *v.Leftovers.Tab)
	}
}
