package server

import (
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func summaryTicket(n, status string, blockedBy ...string) tickets.Ticket {
	return tickets.Ticket{Number: int(n[1] - '0'), Identifier: n, Title: "title " + n, Status: status, BlockedBy: blockedBy}
}

func pausedTicket(n string, reason schema.PauseReason) tickets.Ticket {
	t := summaryTicket(n, string(schema.StatusPaused))
	t.PauseReason = reason
	return t
}

// repairPausedTicket is paused with a repair-class reason.
func repairPausedTicket(n string) tickets.Ticket { return pausedTicket(n, schema.PauseMergeConflict) }

// answerPausedTicket is paused with an answer-class reason.
func answerPausedTicket(n string) tickets.Ticket { return pausedTicket(n, schema.PauseZeroCommit) }

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
	pausedBody := "# t\n\n## Needs Repair\n\n**the `really` long reason**\n\nmore\n"

	cases := []struct {
		name       string
		epic       tickets.Epic
		wantState  EpicState
		wantDone   int
		wantActive []string
		wantPaused []string
		wantReason string
		wantMore   int
		wantDetail string
		wantKind   string
		wantRepair bool
	}{
		{
			name:      "awaiting merge beats everything",
			epic:      tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{summaryTicket("01", "done"), summaryTicket("02", "done")}},
			wantState: EpicStateAwaitingMerge, wantDone: 2,
		},
		{
			name:      "completed but unmerged stays awaiting merge",
			epic:      tickets.Epic{Name: "epic", CompletedAt: time.Unix(50, 0), Tickets: []tickets.Ticket{summaryTicket("01", "done")}},
			wantState: EpicStateAwaitingMerge, wantDone: 1,
		},
		{
			name: "done, unmerged with a stored reason is merge-blocked",
			epic: tickets.Epic{
				Name: "epic", CompletedAt: time.Unix(50, 0), MergeBlockKind: "dirty-trunk", MergeBlockReason: "trunk dirty",
				Tickets: []tickets.Ticket{summaryTicket("01", "done")},
			},
			wantState: EpicStatePaused, wantDone: 1, wantReason: EpicReasonMergeBlocked, wantDetail: "trunk dirty", wantKind: "dirty-trunk",
		},
		{
			name:      "merged epic ignores a stale reason",
			epic:      tickets.Epic{Name: "epic", MergedAt: time.Unix(60, 0), MergeBlockReason: "stale", Tickets: []tickets.Ticket{summaryTicket("01", "done")}},
			wantState: EpicStateFinished, wantDone: 1,
		},
		{
			name:      "merged epic is finished",
			epic:      tickets.Epic{Name: "epic", CompletedAt: time.Unix(50, 0), MergedAt: time.Unix(60, 0), Tickets: []tickets.Ticket{summaryTicket("01", "done")}},
			wantState: EpicStateFinished, wantDone: 1,
		},
		{
			name: "running: claimed with a server claim time",
			epic: tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
				summaryTicket("01", "done"), summaryTicket("02", "claimed"), repairPausedTicket("03"),
			}},
			wantState: EpicStateRunning, wantDone: 1, wantActive: []string{"proj:epic/02"}, wantPaused: []string{"proj:epic/03"}, wantRepair: true,
		},
		{
			name: "claimed without a claim time is runnable, so waiting",
			epic: tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
				summaryTicket("01", "claimed"), answerPausedTicket("03"),
			}},
			wantState: EpicStateWaiting, wantPaused: []string{"proj:epic/03"},
		},
		{
			name: "waiting: an open unblocked ticket",
			epic: tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
				summaryTicket("01", "done"), summaryTicket("04", "open"),
			}},
			wantState: EpicStateWaiting, wantDone: 1,
		},
		{
			name: "paused: nothing runnable, shows the first paused ticket's reason and the rest as more; a draft is not paused",
			epic: tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
				summaryTicket("01", "draft"), repairPausedTicket("03"), summaryTicket("04", "nonsense"),
			}},
			wantState: EpicStatePaused, wantPaused: []string{"proj:epic/01", "proj:epic/03", "proj:epic/04"}, wantRepair: true,
			wantReason: "the really long reason", wantMore: 1,
		},
		{
			name: "drafts only: paused with no reason",
			epic: tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
				summaryTicket("01", "draft"), summaryTicket("02", "draft"),
			}},
			wantState: EpicStatePaused, wantPaused: []string{"proj:epic/01", "proj:epic/02"},
		},
		{
			name: "deadlocked: only blocked tickets, the cycle is the detail",
			epic: tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
				summaryTicket("01", "open", "02"), summaryTicket("02", "open", "01"),
			}},
			wantState:  EpicStatePaused,
			wantReason: EpicReasonDeadlock,
			wantDetail: "proj:epic/01 waits on proj:epic/02; proj:epic/02 waits on proj:epic/01",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for i := range tc.epic.Tickets {
				if tc.epic.Tickets[i].Status == string(schema.StatusPaused) {
					tc.epic.Tickets[i].Body = pausedBody
				}
			}
			got := summarizeEpic(root, tc.epic, claimed, recovering, noLog)
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
			if addrs := summaryAddresses(got.Paused); !equalStrings(addrs, tc.wantPaused) {
				t.Errorf("paused = %v, want %v", addrs, tc.wantPaused)
			}
			if got.HasRepair != tc.wantRepair {
				t.Errorf("has_repair = %v, want %v", got.HasRepair, tc.wantRepair)
			}
			if got.Reason != tc.wantReason {
				t.Errorf("reason = %q, want %q", got.Reason, tc.wantReason)
			}
			if got.More != tc.wantMore || got.Detail != tc.wantDetail {
				t.Errorf("more/detail = %d %q, want %d %q", got.More, got.Detail, tc.wantMore, tc.wantDetail)
			}
			if got.MergeBlockKind != tc.wantKind || got.MergeBlocked != (tc.wantKind != "") {
				t.Errorf("merge block = %v %q, want kind %q", got.MergeBlocked, got.MergeBlockKind, tc.wantKind)
			}
		})
	}
}

func TestSummarizeEpic_PausedDetails(t *testing.T) {
	root := rootRef{Project: "proj", Epic: "epic"}
	repair := repairPausedTicket("03")
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
		}, noLog)
	if len(got.Paused) != 2 {
		t.Fatalf("paused = %+v", got.Paused)
	}
	p := got.Paused[0]
	if p.Title != "title 03" || p.Status != tickets.StatusPausedRepair.Word() || p.Recovery != RecoveryEscalated || p.Reason != "the really long reason" {
		t.Errorf("paused[0] = %+v", p)
	}
	if got.Paused[1].Reason != "cannot read it" {
		t.Errorf("error reason = %q", got.Paused[1].Reason)
	}
}

func noLog(tickets.Ticket) []tickets.LogEvent { return nil }

func TestSummarizeEpic_PausedEntriesCarrySummaryAnswerableAnswered(t *testing.T) {
	root := rootRef{Project: "proj", Epic: "epic"}
	asked := answerPausedTicket("01")
	asked.Body = "## Needs Answer\n\nfallback line\n"
	repair := repairPausedTicket("02")
	repair.Body = "## Needs Repair\n\nthe repair reason\n"
	answered := summaryTicket("03", "open")
	question := func(seq int, summary string) tickets.LogEvent {
		return tickets.LogEvent{Type: tickets.EventPauseQuestion, Seq: seq, Summary: summary}
	}
	logs := map[string][]tickets.LogEvent{
		"01": {question(1, "which one?")},
		"03": {question(1, "old"), {Type: tickets.EventPauseAnswer, Seq: 2, Question: 1}},
	}
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{asked, repair, answered}}
	got := summarizeEpic(root, epic,
		func(string) (time.Time, bool) { return time.Time{}, false },
		func(string) string { return "" },
		func(tk tickets.Ticket) []tickets.LogEvent { return logs[tk.Identifier] })
	if len(got.Paused) != 3 {
		t.Fatalf("paused = %+v", got.Paused)
	}
	if p := got.Paused[0]; p.Summary != "which one?" || !p.Answerable || p.Answered {
		t.Errorf("asked = %+v", p)
	}
	if p := got.Paused[1]; p.Summary != "the repair reason" || p.Answerable || p.Answered {
		t.Errorf("repair = %+v", p)
	}
	if p := got.Paused[2]; p.Answerable || !p.Answered {
		t.Errorf("answered = %+v", p)
	}
}

func TestSummarizeEpic_OneOffsRootNeverAwaitsMergeAndCountsNothing(t *testing.T) {
	none := func(string) (time.Time, bool) { return time.Time{}, false }
	noRecovery := func(string) string { return "" }
	root := rootRef{Project: "proj", Epic: OneOffsEpic}

	allDone := tickets.Epic{Name: OneOffsEpic, Tickets: []tickets.Ticket{summaryTicket("01", "done")}}
	got := summarizeEpic(root, allDone, none, noRecovery, noLog)
	if got.State == EpicStateAwaitingMerge || got.Done != 0 || got.Total != 0 {
		t.Errorf("all-done one-offs = state %q done %d total %d", got.State, got.Done, got.Total)
	}

	broken := tickets.Epic{Name: OneOffsEpic, Tickets: []tickets.Ticket{
		summaryTicket("01", "done"), repairPausedTicket("02"), answerPausedTicket("03"),
	}}
	got = summarizeEpic(root, broken, none, noRecovery, noLog)
	if got.State != EpicStatePaused || len(got.Paused) != 2 {
		t.Errorf("one-offs with paused tickets = state %q paused %+v", got.State, got.Paused)
	}
}

func mergeSummaryTicket(n, status string) tickets.Ticket {
	t := summaryTicket(n, status)
	t.Type = string(schema.TypeMerge)
	return t
}

func TestSummarizeEpic_MergingState(t *testing.T) {
	noRecovery := func(string) string { return "" }
	claimedAll := func(string) (time.Time, bool) { return time.Unix(100, 0), true }
	pausedBody := "# t\n\n## Paused\n\n**why**\n"
	pausedMerge := mergeSummaryTicket("02", "paused")
	pausedMerge.PauseReason = schema.PauseMergeConflict
	pausedMerge.Body = pausedBody

	cases := []struct {
		name       string
		epic       string
		tickets    []tickets.Ticket
		wantState  EpicState
		wantActive []string
	}{
		{"open merge ticket", "epic", []tickets.Ticket{summaryTicket("01", "done"), mergeSummaryTicket("02", "open")}, EpicStateMerging, nil},
		{"claimed merge ticket stays active", "epic", []tickets.Ticket{summaryTicket("01", "done"), mergeSummaryTicket("02", "claimed")}, EpicStateMerging, []string{"proj:epic/02"}},
		{"cancelled work does not block merging", "epic", []tickets.Ticket{summaryTicket("01", "cancelled"), mergeSummaryTicket("02", "open")}, EpicStateMerging, nil},
		{"paused merge ticket keeps paused", "epic", []tickets.Ticket{summaryTicket("01", "done"), pausedMerge}, EpicStatePaused, nil},
		{"unfinished work beside a merge ticket is not merging", "epic", []tickets.Ticket{summaryTicket("01", "open"), mergeSummaryTicket("02", "open")}, EpicStateWaiting, nil},
		{"one-offs epic is never merging", OneOffsEpic, []tickets.Ticket{summaryTicket("01", "done"), mergeSummaryTicket("02", "open")}, EpicStateWaiting, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := rootRef{Project: "proj", Epic: tc.epic}
			claimed := func(string) (time.Time, bool) { return time.Time{}, false }
			if tc.wantActive != nil {
				claimed = claimedAll
			}
			got := summarizeEpic(root, tickets.Epic{Name: tc.epic, Tickets: tc.tickets}, claimed, noRecovery, noLog)
			if got.State != tc.wantState {
				t.Fatalf("state = %q, want %q", got.State, tc.wantState)
			}
			var active []string
			for _, a := range got.Active {
				active = append(active, a.Address)
			}
			if !slices.Equal(active, tc.wantActive) {
				t.Errorf("active = %v, want %v", active, tc.wantActive)
			}
		})
	}
}

// The Deadlocked state is deadlockKind's blocked cycle, nothing more.
func TestSummarizeEpic_DeadlockedAgreesWithDeadlockKind(t *testing.T) {
	root := rootRef{Project: "proj", Epic: "epic"}
	none := func(string) (time.Time, bool) { return time.Time{}, false }
	noRecovery := func(string) string { return "" }
	for _, statuses := range [][]string{
		{"open"}, {"done", "open"}, {"paused"}, {"draft", "open"}, {"nonsense"}, {"claimed"}, {"cancelled", "done"},
	} {
		var ts []tickets.Ticket
		for i, st := range statuses {
			ts = append(ts, summaryTicket("0"+string(rune('1'+i)), st))
		}
		// A last ticket blocked on itself keeps blocked-only shapes honest.
		epic := tickets.Epic{Name: "epic", Tickets: ts}
		kind, stuck := deadlockKind(epic)
		got := summarizeEpic(root, epic, none, noRecovery, noLog)
		if (got.State == EpicStatePaused) != stuck {
			t.Errorf("%v: state %q, deadlockKind %q/%v", statuses, got.State, kind, stuck)
		}
		if want := stuck && kind == events.BlockedCycle; (got.Reason == EpicReasonDeadlock) != want {
			t.Errorf("%v: reason %q, deadlockKind %q/%v", statuses, got.Reason, kind, stuck)
		}
	}
}

func TestDeadlockReason_PausedAndCycle(t *testing.T) {
	root := rootRef{Project: "proj", Epic: "epic"}
	paused := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{answerPausedTicket("01"), summaryTicket("02", "draft")}}
	if got, want := deadlockReason(root, paused, events.AllPaused), "every remaining ticket is paused: proj:epic/01, proj:epic/02"; got != want {
		t.Errorf("paused reason = %q, want %q", got, want)
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
