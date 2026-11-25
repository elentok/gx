package epics

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/tickets/ticketlog"
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

func TestNoClaims_ReportsNoClaimForAnyAddress(t *testing.T) {
	for _, addr := range []string{"", "gx:e/1", "anything"} {
		if at, ok := NoClaims(addr); ok || !at.IsZero() {
			t.Fatalf("NoClaims(%q) = %v, %v; want zero, false", addr, at, ok)
		}
	}
}

func TestSummarizeEpic_StateOrderAndDetails(t *testing.T) {
	t.Parallel()
	root := queue.RootRef{Project: "proj", Epic: "epic"}
	claimTime := time.Unix(100, 0)
	claimed := func(addr string) (time.Time, bool) {
		if addr == "proj:epic/02" {
			return claimTime, true
		}
		return time.Time{}, false
	}
	recovering := func(addr string) string {
		if addr == "proj:epic/03" {
			return wire.RecoveryPending
		}
		return ""
	}
	pausedBody := "# t\n\n## Needs Repair\n\n**the `really` long reason**\n\nmore\n"

	cases := []struct {
		name       string
		epic       tickets.Epic
		wantState  wire.EpicState
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
			wantState: wire.EpicStateAwaitingMerge, wantDone: 2,
		},
		{
			name:      "completed but unmerged stays awaiting merge",
			epic:      tickets.Epic{Name: "epic", CompletedAt: time.Unix(50, 0), Tickets: []tickets.Ticket{summaryTicket("01", "done")}},
			wantState: wire.EpicStateAwaitingMerge, wantDone: 1,
		},
		{
			name: "done, unmerged with a stored reason is merge-blocked",
			epic: tickets.Epic{
				Name: "epic", CompletedAt: time.Unix(50, 0), MergeBlockKind: "dirty-trunk", MergeBlockReason: "trunk dirty",
				Tickets: []tickets.Ticket{summaryTicket("01", "done")},
			},
			wantState: wire.EpicStatePaused, wantDone: 1, wantReason: wire.EpicReasonMergeBlocked, wantDetail: "trunk dirty", wantKind: "dirty-trunk",
		},
		{
			name:      "merged epic ignores a stale reason",
			epic:      tickets.Epic{Name: "epic", MergedAt: time.Unix(60, 0), MergeBlockReason: "stale", Tickets: []tickets.Ticket{summaryTicket("01", "done")}},
			wantState: wire.EpicStateFinished, wantDone: 1,
		},
		{
			name:      "merged epic is finished",
			epic:      tickets.Epic{Name: "epic", CompletedAt: time.Unix(50, 0), MergedAt: time.Unix(60, 0), Tickets: []tickets.Ticket{summaryTicket("01", "done")}},
			wantState: wire.EpicStateFinished, wantDone: 1,
		},
		{
			name: "running: claimed with a server claim time",
			epic: tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
				summaryTicket("01", "done"), summaryTicket("02", "claimed"), repairPausedTicket("03"),
			}},
			wantState: wire.EpicStateRunning, wantDone: 1, wantActive: []string{"proj:epic/02"}, wantPaused: []string{"proj:epic/03"}, wantRepair: true,
		},
		{
			name: "claimed without a claim time is runnable, so waiting",
			epic: tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
				summaryTicket("01", "claimed"), answerPausedTicket("03"),
			}},
			wantState: wire.EpicStateWaiting, wantPaused: []string{"proj:epic/03"},
		},
		{
			name: "waiting: an open unblocked ticket",
			epic: tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
				summaryTicket("01", "done"), summaryTicket("04", "open"),
			}},
			wantState: wire.EpicStateWaiting, wantDone: 1,
		},
		{
			name: "paused: nothing runnable, shows the first paused ticket's reason and the rest as more; a draft is not paused",
			epic: tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
				summaryTicket("01", "draft"), repairPausedTicket("03"), summaryTicket("04", "nonsense"),
			}},
			wantState: wire.EpicStatePaused, wantPaused: []string{"proj:epic/01", "proj:epic/03", "proj:epic/04"}, wantRepair: true,
			wantReason: "the really long reason", wantMore: 1,
		},
		{
			name: "drafts only: paused with no reason",
			epic: tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
				summaryTicket("01", "draft"), summaryTicket("02", "draft"),
			}},
			wantState: wire.EpicStatePaused, wantPaused: []string{"proj:epic/01", "proj:epic/02"},
		},
		{
			name: "deadlocked: only blocked tickets, the cycle is the detail",
			epic: tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{
				summaryTicket("01", "open", "02"), summaryTicket("02", "open", "01"),
			}},
			wantState:  wire.EpicStatePaused,
			wantReason: wire.EpicReasonDeadlock,
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
			got := summarizeEpic(root, tc.epic, "merge", claimed, recovering, noLog)
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

func TestSummarizeEpic_MergeBlockActionsFollowTheVerbTable(t *testing.T) {
	t.Parallel()
	root := queue.RootRef{Project: "proj", Epic: "epic"}
	want := map[string][]string{
		"dirty-trunk":   {"stash-merge", "merge"},
		"merge-error":   {"merge"},
		"trunk-missing": {"merge"},
		"needs-rebase":  {"merge"},

		"pr-foreign-conflict": {"merge"},
		"pr-dirty-trunk":      {"stash-merge", "merge"},
		"pr-trunk-update":     {"merge"},
	}
	for kind, actions := range want {
		e := tickets.Epic{Name: "epic", CompletedAt: time.Unix(50, 0), MergeBlockKind: kind, MergeBlockReason: "why", Tickets: []tickets.Ticket{summaryTicket("01", "done")}}
		got := summarizeEpic(root, e, "merge", func(string) (time.Time, bool) { return time.Time{}, false }, func(string) string { return "" }, func(tickets.Ticket) []ticketlog.LogEvent { return nil })
		if !equalStrings(got.Actions, actions) {
			t.Errorf("%s actions = %v, want %v", kind, got.Actions, actions)
		}
	}
}

func TestSummarizeEpic_PausedDetails(t *testing.T) {
	t.Parallel()
	root := queue.RootRef{Project: "proj", Epic: "epic"}
	repair := repairPausedTicket("03")
	repair.Body = "## Needs Repair\n\n**the `really` long reason**\n\nmore\n"
	broken := summaryTicket("04", "nonsense")
	broken.ReadErr = "cannot read it"
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{repair, broken}}
	got := summarizeEpic(root, epic, "merge",
		func(string) (time.Time, bool) { return time.Time{}, false },
		func(addr string) string {
			if addr == "proj:epic/03" {
				return wire.RecoveryEscalated
			}
			return ""
		}, noLog)
	if len(got.Paused) != 2 {
		t.Fatalf("paused = %+v", got.Paused)
	}
	p := got.Paused[0]
	if p.Title != "title 03" || p.Status != tickets.StatusPausedRepair.Word() || p.Recovery != wire.RecoveryEscalated || p.Reason != "the really long reason" {
		t.Errorf("paused[0] = %+v", p)
	}
	if got.Paused[1].Reason != "cannot read it" {
		t.Errorf("error reason = %q", got.Paused[1].Reason)
	}
}

func noLog(tickets.Ticket) []ticketlog.LogEvent { return nil }

func TestSummarizeEpic_PausedEntriesCarrySummaryAnswerableAnswered(t *testing.T) {
	t.Parallel()
	root := queue.RootRef{Project: "proj", Epic: "epic"}
	asked := answerPausedTicket("01")
	asked.Body = "## Needs Answer\n\nfallback line\n"
	repair := repairPausedTicket("02")
	repair.Body = "## Needs Repair\n\nthe repair reason\n"
	answered := summaryTicket("03", "open")
	question := func(seq int, summary string) ticketlog.LogEvent {
		return ticketlog.LogEvent{Type: ticketlog.EventPauseQuestion, Seq: seq, Summary: summary}
	}
	logs := map[string][]ticketlog.LogEvent{
		"01": {question(1, "which one?")},
		"03": {question(1, "old"), {Type: ticketlog.EventPauseAnswer, Seq: 2, Question: 1}},
	}
	epic := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{asked, repair, answered}}
	got := summarizeEpic(root, epic, "merge",
		func(string) (time.Time, bool) { return time.Time{}, false },
		func(string) string { return "" },
		func(tk tickets.Ticket) []ticketlog.LogEvent { return logs[tk.Identifier] })
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
	t.Parallel()
	none := func(string) (time.Time, bool) { return time.Time{}, false }
	noRecovery := func(string) string { return "" }
	root := queue.RootRef{Project: "proj", Epic: queue.OneOffsEpic}

	allDone := tickets.Epic{Name: queue.OneOffsEpic, Tickets: []tickets.Ticket{summaryTicket("01", "done")}}
	got := summarizeEpic(root, allDone, "merge", none, noRecovery, noLog)
	if got.State == wire.EpicStateAwaitingMerge || got.Done != 0 || got.Total != 0 {
		t.Errorf("all-done one-offs = state %q done %d total %d", got.State, got.Done, got.Total)
	}

	broken := tickets.Epic{Name: queue.OneOffsEpic, Tickets: []tickets.Ticket{
		summaryTicket("01", "done"), repairPausedTicket("02"), answerPausedTicket("03"),
	}}
	got = summarizeEpic(root, broken, "merge", none, noRecovery, noLog)
	if got.State != wire.EpicStatePaused || len(got.Paused) != 2 {
		t.Errorf("one-offs with paused tickets = state %q paused %+v", got.State, got.Paused)
	}
}

func mergeSummaryTicket(n, status string) tickets.Ticket {
	t := summaryTicket(n, status)
	t.Type = string(schema.TypeMerge)
	return t
}

func TestSummarizeEpic_MergingState(t *testing.T) {
	t.Parallel()
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
		wantState  wire.EpicState
		wantActive []string
	}{
		{"open merge ticket", "epic", []tickets.Ticket{summaryTicket("01", "done"), mergeSummaryTicket("02", "open")}, wire.EpicStateMerging, nil},
		{"claimed merge ticket stays active", "epic", []tickets.Ticket{summaryTicket("01", "done"), mergeSummaryTicket("02", "claimed")}, wire.EpicStateMerging, []string{"proj:epic/02"}},
		{"cancelled work does not block merging", "epic", []tickets.Ticket{summaryTicket("01", "cancelled"), mergeSummaryTicket("02", "open")}, wire.EpicStateMerging, nil},
		{"paused merge ticket keeps paused", "epic", []tickets.Ticket{summaryTicket("01", "done"), pausedMerge}, wire.EpicStatePaused, nil},
		{"unfinished work beside a merge ticket is not merging", "epic", []tickets.Ticket{summaryTicket("01", "open"), mergeSummaryTicket("02", "open")}, wire.EpicStateWaiting, nil},
		{"one-offs epic is never merging", queue.OneOffsEpic, []tickets.Ticket{summaryTicket("01", "done"), mergeSummaryTicket("02", "open")}, wire.EpicStateWaiting, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := queue.RootRef{Project: "proj", Epic: tc.epic}
			claimed := func(string) (time.Time, bool) { return time.Time{}, false }
			if tc.wantActive != nil {
				claimed = claimedAll
			}
			got := summarizeEpic(root, tickets.Epic{Name: tc.epic, Tickets: tc.tickets}, "merge", claimed, noRecovery, noLog)
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

// The Deadlocked state is StateOf's cycle, nothing more.
func TestSummarizeEpic_DeadlockedAgreesWithStateOf(t *testing.T) {
	t.Parallel()
	root := queue.RootRef{Project: "proj", Epic: "epic"}
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
		state := StateOf(epic)
		stuck := state == StatePaused || state == StateCycle
		got := summarizeEpic(root, epic, "merge", none, noRecovery, noLog)
		if (got.State == wire.EpicStatePaused) != stuck {
			t.Errorf("%v: state %q, StateOf %v", statuses, got.State, state)
		}
		if want := state == StateCycle; (got.Reason == wire.EpicReasonDeadlock) != want {
			t.Errorf("%v: reason %q, StateOf %v", statuses, got.Reason, state)
		}
	}
}

// diskEpic and diskIssue mirror rigs.WriteEpicFiles and rigs.IssueMD, which
// this package cannot import without a cycle.
func diskEpic(t *testing.T, store, epic, ticketMD string, issues map[string]string) {
	t.Helper()
	dir := filepath.Join(store, "proj", epic)
	if err := os.MkdirAll(filepath.Join(dir, "issues"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(store, "proj", "project.json"): `{"name":"proj"}`,
		filepath.Join(dir, "ticket.md"):              ticketMD,
	}
	for name, content := range issues {
		files[filepath.Join(dir, "issues", name)] = content
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func diskIssue(id, status, extra, title string) string {
	return "---\nid: \"" + id + "\"\nstatus: " + status + "\ntype: implement\n" + extra + "---\n\n# " + title + "\n"
}

// summaryOnDisk loads store/proj through the hold resolver and summarizes
// the named epic.
func summaryOnDisk(t *testing.T, store, name string) (tickets.Epic, wire.EpicSummary) {
	t.Helper()
	all, err := tickets.LoadResolved("proj", filepath.Join(store, "proj"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range all {
		if e.Name == name {
			root := queue.RootRef{Project: "proj", Epic: name}
			none := func(string) (time.Time, bool) { return time.Time{}, false }
			return e, summarizeEpic(root, e, "merge", none, func(string) string { return "" }, noLog)
		}
	}
	t.Fatalf("epic %q not loaded", name)
	return tickets.Epic{}, wire.EpicSummary{}
}

func TestSummarizeEpic_WaitingOnEpicUntilBlockerReleased(t *testing.T) {
	t.Parallel()
	issues := map[string]string{"01-a.md": diskIssue("01", "open", "", "A")}
	for _, tc := range []struct {
		name     string
		project  string // project.json written after the epics, "" keeps the default
		released string // blocker ticket.md once released
	}{
		{"on-merge", "", "---\nstatus: done\nmerged_at: 2026-01-02T11:00:00Z\n---\n"},
		{"on-done", `{"landing":"on-done"}`, "---\nstatus: open\ncompleted_at: 2026-01-02T11:00:00Z\n---\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := t.TempDir()
			write := func(blocker string, blockerIssues map[string]string) {
				diskEpic(t, store, "blocker", blocker, blockerIssues)
				diskEpic(t, store, "waiter", "---\nstatus: open\nblocked_by: [blocker]\n---\n", issues)
				if tc.project != "" {
					if err := os.WriteFile(filepath.Join(store, "proj", "project.json"), []byte(tc.project), 0o644); err != nil {
						t.Fatal(err)
					}
				}
			}
			write("---\nstatus: open\n---\n", issues)
			e, sum := summaryOnDisk(t, store, "waiter")
			if got := StateOf(e); got != StateWaiting {
				t.Errorf("StateOf = %v, want StateWaiting", got)
			}
			if sum.State != wire.EpicStateWaitingOnEpic {
				t.Errorf("state = %q, want %q", sum.State, wire.EpicStateWaitingOnEpic)
			}
			write(tc.released, map[string]string{"01-a.md": diskIssue("01", "done", "", "A")})
			e, sum = summaryOnDisk(t, store, "waiter")
			if got := StateOf(e); got != StateRunnable {
				t.Errorf("released: StateOf = %v, want StateRunnable", got)
			}
			if sum.State == wire.EpicStateWaitingOnEpic || sum.State == wire.EpicStatePaused {
				t.Errorf("released: state = %q", sum.State)
			}
		})
	}
}

func TestSummarizeEpic_RealCycleIsDeadlockedNotErrored(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	cycle := map[string]string{
		"01-a.md": diskIssue("01", "open", "blocked_by: [\"02\"]\n", "A"),
		"02-b.md": diskIssue("02", "open", "blocked_by: [\"01\"]\n", "B"),
	}
	diskEpic(t, store, "blocker", "---\nstatus: open\n---\n", map[string]string{"01-a.md": diskIssue("01", "open", "", "A")})
	diskEpic(t, store, "waiter", "---\nstatus: open\nblocked_by: [blocker]\n---\n", cycle)

	e, sum := summaryOnDisk(t, store, "waiter")
	if got := StateOf(e); got != StateCycle {
		t.Errorf("StateOf = %v, want StateCycle", got)
	}
	if sum.State != wire.EpicStatePaused || sum.Reason != wire.EpicReasonDeadlock {
		t.Errorf("summary = %q / %q, want paused / deadlock", sum.State, sum.Reason)
	}

	cycle["03-c.md"] = diskIssue("03", "paused", "pause_reason: zero-commit\n", "C")
	diskEpic(t, store, "waiter", "---\nstatus: open\nblocked_by: [blocker]\n---\n", cycle)
	e, sum = summaryOnDisk(t, store, "waiter")
	if got := StateOf(e); got != StatePaused {
		t.Errorf("with a paused ticket: StateOf = %v, want StatePaused", got)
	}
	if sum.State != wire.EpicStatePaused || sum.Reason == wire.EpicReasonDeadlock {
		t.Errorf("with a paused ticket: summary = %q / %q, want paused with a ticket reason", sum.State, sum.Reason)
	}
}

func TestPausedAndCycleReason(t *testing.T) {
	t.Parallel()
	root := queue.RootRef{Project: "proj", Epic: "epic"}
	paused := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{answerPausedTicket("01"), summaryTicket("02", "draft")}}
	if got, want := PausedReason(root, paused), "every remaining ticket is paused: proj:epic/01, proj:epic/02"; got != want {
		t.Errorf("paused reason = %q, want %q", got, want)
	}
	cycle := tickets.Epic{Name: "epic", Tickets: []tickets.Ticket{summaryTicket("01", "open", "02"), summaryTicket("02", "open", "01")}}
	want := "proj:epic/01 waits on proj:epic/02; proj:epic/02 waits on proj:epic/01"
	if got := CycleReason(root, cycle); got != want {
		t.Errorf("cycle reason = %q, want %q", got, want)
	}
}

func summaryAddresses(list []wire.SummaryTicket) []string {
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
