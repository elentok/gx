package server

import (
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/tickets"
)

func summaryTicket(n, status string, blockedBy ...string) tickets.Ticket {
	return tickets.Ticket{Number: int(n[1] - '0'), Identifier: n, Title: "title " + n, Status: status, BlockedBy: blockedBy}
}

func TestSummarizeEpic_StateOrderAndDetails(t *testing.T) {
	root := rootRef{Project: "proj", Epic: "epic"}
	claimTime := time.Unix(100, 0)
	claimed := func(addr string) (time.Time, bool) {
		if addr == "proj:epic/02" {
			return claimTime, true
		}
		return time.Time{}, false
	}
	recovering := func(addr string) string {
		if addr == "proj:epic/03" {
			return RecoveryPending
		}
		return ""
	}
	parkedBody := "# t\n\n## Needs Repair\n\n**the `really` long reason**\n\nmore\n"

	cases := []struct {
		name       string
		epic       tickets.Epic
		wantState  EpicState
		wantDone   int
		wantActive []string
		wantParked []string
		wantReason string
		wantRepair bool
	}{
		{
			name:      "awaiting merge beats everything",
			epic:      tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{summaryTicket("01", "done"), summaryTicket("02", "done")}},
			wantState: EpicStateAwaitingMerge, wantDone: 2,
		},
		{
			name: "running: claimed with a server claim time",
			epic: tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
				summaryTicket("01", "done"), summaryTicket("02", "claimed"), summaryTicket("03", "needs-repair"),
			}},
			wantState: EpicStateRunning, wantDone: 1, wantActive: []string{"proj:epic/02"}, wantParked: []string{"proj:epic/03"}, wantRepair: true,
		},
		{
			name: "claimed without a claim time is runnable, so waiting",
			epic: tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
				summaryTicket("01", "claimed"), summaryTicket("03", "needs-answer"),
			}},
			wantState: EpicStateWaiting, wantParked: []string{"proj:epic/03"},
		},
		{
			name: "waiting: an open unblocked ticket",
			epic: tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
				summaryTicket("01", "done"), summaryTicket("04", "open"),
			}},
			wantState: EpicStateWaiting, wantDone: 1,
		},
		{
			name: "parked: nothing runnable, draft and error count as parked",
			epic: tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
				summaryTicket("01", "draft"), summaryTicket("03", "needs-repair"), summaryTicket("04", "nonsense"),
			}},
			wantState: EpicStateParked, wantParked: []string{"proj:epic/01", "proj:epic/03", "proj:epic/04"}, wantRepair: true,
		},
		{
			name: "deadlocked: only blocked tickets, reason names what they wait on",
			epic: tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
				summaryTicket("01", "open", "02"), summaryTicket("02", "open", "01"),
			}},
			wantState:  EpicStateDeadlocked,
			wantReason: "proj:epic/01 waits on proj:epic/02; proj:epic/02 waits on proj:epic/01",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for i := range tc.epic.Tickets {
				if tc.epic.Tickets[i].Status == "needs-repair" {
					tc.epic.Tickets[i].Body = parkedBody
				}
			}
			got := summarizeEpic(root, tc.epic, claimed, recovering)
			if got.State != tc.wantState {
				t.Fatalf("state = %q, want %q", got.State, tc.wantState)
			}
			if got.Address != "proj:epic" || got.Name != "epic" || got.Project != "proj" {
				t.Errorf("identity = %q %q %q", got.Address, got.Name, got.Project)
			}
			if got.Done != tc.wantDone || got.Total != len(tc.epic.Tickets) {
				t.Errorf("done/total = %d/%d", got.Done, got.Total)
			}
			if addrs := summaryAddresses(got.Active); !equalStrings(addrs, tc.wantActive) {
				t.Errorf("active = %v, want %v", addrs, tc.wantActive)
			}
			if addrs := summaryAddresses(got.Parked); !equalStrings(addrs, tc.wantParked) {
				t.Errorf("parked = %v, want %v", addrs, tc.wantParked)
			}
			if got.HasRepair != tc.wantRepair {
				t.Errorf("has_repair = %v, want %v", got.HasRepair, tc.wantRepair)
			}
			if got.Reason != tc.wantReason {
				t.Errorf("reason = %q, want %q", got.Reason, tc.wantReason)
			}
		})
	}
}

func TestSummarizeEpic_ParkedDetails(t *testing.T) {
	root := rootRef{Project: "proj", Epic: "epic"}
	repair := summaryTicket("03", "needs-repair")
	repair.Body = "## Needs Repair\n\n**the `really` long reason**\n\nmore\n"
	broken := summaryTicket("04", "nonsense")
	broken.ReadErr = "cannot read it"
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{repair, broken}}
	got := summarizeEpic(root, epic,
		func(string) (time.Time, bool) { return time.Time{}, false },
		func(addr string) string {
			if addr == "proj:epic/03" {
				return RecoveryEscalated
			}
			return ""
		})
	if len(got.Parked) != 2 {
		t.Fatalf("parked = %+v", got.Parked)
	}
	p := got.Parked[0]
	if p.Title != "title 03" || p.Status != "needs-repair" || p.Recovery != RecoveryEscalated || p.Reason != "the really long reason" {
		t.Errorf("parked[0] = %+v", p)
	}
	if got.Parked[1].Reason != "cannot read it" {
		t.Errorf("error reason = %q", got.Parked[1].Reason)
	}
}

// The Deadlocked state is deadlockKind's blocked cycle, nothing more.
func TestSummarizeEpic_DeadlockedAgreesWithDeadlockKind(t *testing.T) {
	root := rootRef{Project: "proj", Epic: "epic"}
	none := func(string) (time.Time, bool) { return time.Time{}, false }
	noRecovery := func(string) string { return "" }
	for _, statuses := range [][]string{
		{"open"}, {"done", "open"}, {"needs-answer"}, {"draft", "open"}, {"nonsense"}, {"claimed"}, {"cancelled", "done"},
	} {
		var ts []tickets.Ticket
		for i, st := range statuses {
			ts = append(ts, summaryTicket("0"+string(rune('1'+i)), st))
		}
		// A last ticket blocked on itself keeps blocked-only shapes honest.
		epic := tickets.Epic{Name: "epic", Tickets: ts}
		kind, stuck := deadlockKind(epic)
		got := summarizeEpic(root, epic, none, noRecovery)
		if want := stuck && kind == events.BlockedCycle; (got.State == EpicStateDeadlocked) != want {
			t.Errorf("%v: state %q, deadlockKind %q/%v", statuses, got.State, kind, stuck)
		}
		if want := stuck && kind == events.AllParked; (got.State == EpicStateParked) != want {
			t.Errorf("%v: state %q, deadlockKind %q/%v", statuses, got.State, kind, stuck)
		}
	}
}

func TestDeadlockReason_ParkedAndCycle(t *testing.T) {
	root := rootRef{Project: "proj", Epic: "epic"}
	parked := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{summaryTicket("01", "needs-answer"), summaryTicket("02", "draft")}}
	if got, want := deadlockReason(root, parked, events.AllParked), "every remaining ticket is parked: proj:epic/01, proj:epic/02"; got != want {
		t.Errorf("parked reason = %q, want %q", got, want)
	}
	first := summaryTicket("01", "open", "02")
	first.ExternalBlockers = []string{"other:epic2/07"}
	cycle := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{first, summaryTicket("02", "open", "01")}}
	want := "proj:epic/01 waits on other:epic2/07, proj:epic/02; proj:epic/02 waits on proj:epic/01"
	if got := deadlockReason(root, cycle, events.BlockedCycle); got != want {
		t.Errorf("cycle reason = %q, want %q", got, want)
	}
}

func summaryAddresses(list []SummaryTicket) []string {
	var out []string
	for _, x := range list {
		out = append(out, x.Address)
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
