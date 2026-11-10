package epics

import (
	"testing"
	"time"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func typed(n, status string, typ schema.TicketType) tickets.Ticket {
	t := summaryTicket(n, status)
	t.Type = string(typ)
	return t
}

func TestPRStates_SummaryListAndNeedsYou(t *testing.T) {
	t.Parallel()
	done := summaryTicket("01", "done")
	root := queue.RootRef{Project: "proj", Epic: "epic"}
	none := func(string) (time.Time, bool) { return time.Time{}, false }
	noRecovery := func(string) string { return "" }
	cases := []struct {
		name       string
		epic       tickets.Epic
		delivery   string
		wantState  wire.EpicState
		wantList   wire.EpicListState
		wantPill   string
		wantSect   wire.CardSection
		wantWords  string
		wantReason string
	}{
		{"awaiting pr", tickets.Epic{Tickets: []tickets.Ticket{done}}, "pr", wire.EpicStateAwaitingPR, wire.EpicListAwaitingPR, PillAwaitingPR, wire.SectionNeedsYou, "Implemented", ""},
		{"writing pr text", tickets.Epic{Tickets: []tickets.Ticket{done, typed("02", "open", schema.TypePRText)}}, "pr", wire.EpicStateWritingPRText, wire.EpicListWritingPRText, "", wire.SectionImplementing, "Writing PR text", ""},
		{"fixing ci", tickets.Epic{PRNumber: 3, Tickets: []tickets.Ticket{done, typed("02", "open", schema.TypeCIFix)}}, "pr", wire.EpicStateFixingCI, wire.EpicListFixingCI, "", wire.SectionImplementing, "", ""},
		{"pr open", tickets.Epic{PRNumber: 3, Tickets: []tickets.Ticket{done}}, "merge", wire.EpicStatePROpen, wire.EpicListPROpen, "", wire.SectionImplementing, "Implemented", ""},
		{"ci failing", tickets.Epic{PRNumber: 3, MergeBlockKind: "ci-failing", MergeBlockReason: "lint is red", Tickets: []tickets.Ticket{done}}, "pr", wire.EpicStatePaused, wire.EpicListCIFailing, PillCIFailing, wire.SectionNeedsYou, "Implemented", "lint is red"},
		{"pr text failed", tickets.Epic{MergeBlockKind: "pr-text-failed", MergeBlockReason: "no text", Tickets: []tickets.Ticket{done}}, "pr", wire.EpicStatePaused, wire.EpicListPRTextFailed, PillPRTextFailed, wire.SectionNeedsYou, "Implemented", "no text"},
		{"awaiting merge unchanged", tickets.Epic{Tickets: []tickets.Ticket{done}}, "merge", wire.EpicStateAwaitingMerge, wire.EpicListAwaitingMerge, "", wire.SectionImplementing, "Implemented", ""},
		{"merge blocked unchanged", tickets.Epic{MergeBlockKind: "needs-rebase", MergeBlockReason: "behind", Tickets: []tickets.Ticket{done}}, "merge", wire.EpicStatePaused, wire.EpicListMergeBlocked, PillMergeBlocked, wire.SectionNeedsYou, "Implemented", "behind"},
		{"merging unchanged", tickets.Epic{Tickets: []tickets.Ticket{done, mergeSummaryTicket("02", "open")}}, "pr", wire.EpicStateMerging, wire.EpicListMerging, "", wire.SectionImplementing, "Merging...", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := tc.epic
			e.Name = "epic"
			sum := summarizeEpic(root, e, tc.delivery, none, noRecovery, noLog)
			if sum.State != tc.wantState {
				t.Fatalf("state = %q, want %q", sum.State, tc.wantState)
			}
			list := epicListState(e, false, false, sum.State)
			if list != tc.wantList {
				t.Fatalf("list state = %q, want %q", list, tc.wantList)
			}
			if got := loopCardSection(list); got != tc.wantSect {
				t.Errorf("section = %q, want %q", got, tc.wantSect)
			}
			if tc.wantWords != "" {
				if got := loopStatusWords(list, 0); got != tc.wantWords {
					t.Errorf("words = %q, want %q", got, tc.wantWords)
				}
			}
			n, ok := (&Epics{Deps{AutoFFMerge: func(string) bool { return true }}}).loopNeedsYou(root, e, list)
			if ok != (tc.wantSect == wire.SectionNeedsYou) || n.Pill != tc.wantPill && ok && tc.wantPill != "" {
				t.Errorf("needs you = %+v (%v), want pill %q", n, ok, tc.wantPill)
			}
			if tc.wantReason != "" && n.Reason != tc.wantReason {
				t.Errorf("reason = %q, want %q", n.Reason, tc.wantReason)
			}
		})
	}
}

func TestPRBlockedSummary_OwnReasonAndTryAgain(t *testing.T) {
	t.Parallel()
	root := queue.RootRef{Project: "proj", Epic: "epic"}
	for kind, reason := range map[string]string{"ci-failing": wire.EpicReasonCIFailing, "pr-text-failed": wire.EpicReasonPRTextFailed} {
		e := tickets.Epic{Name: "epic", MergeBlockKind: kind, MergeBlockReason: "why", Tickets: []tickets.Ticket{summaryTicket("01", "done")}}
		if EpicMergeBlocked(e) {
			t.Errorf("%s: EpicMergeBlocked = true", kind)
		}
		sum := summarizeEpic(root, e, "pr", func(string) (time.Time, bool) { return time.Time{}, false }, func(string) string { return "" }, noLog)
		if sum.Reason != reason || sum.Detail != "why" || sum.MergeBlocked || len(sum.Actions) != 1 || sum.Actions[0] != "try-again" {
			t.Errorf("%s: summary = %+v", kind, sum)
		}
	}
}

func TestPRStates_OneOffsNeverEnter(t *testing.T) {
	t.Parallel()
	root := queue.RootRef{Project: "proj", Epic: queue.OneOffsEpic}
	e := tickets.Epic{Name: queue.OneOffsEpic, PRNumber: 3, Tickets: []tickets.Ticket{summaryTicket("01", "done"), typed("02", "open", schema.TypePRText)}}
	if got := EpicStateFor(root, e, "pr", func(string) (time.Time, bool) { return time.Time{}, false }); got != wire.EpicStateWaiting {
		t.Errorf("state = %q, want waiting", got)
	}
}
