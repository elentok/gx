package epics

import (
	"testing"
	"time"

	"github.com/elentok/gx/server/epics/prstate"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func kinds(as []wire.CardAction) []wire.CardActionKind {
	var out []wire.CardActionKind
	for _, a := range as {
		out = append(out, a.Kind)
	}
	return out
}

func TestDecoratePR_WordsActionsAndInfo(t *testing.T) {
	t.Parallel()
	root := queue.RootRef{Project: "proj", Epic: "epic"}
	open := wire.CardAction{Kind: wire.CardActionOpenEpic, Label: "Open epic"}
	pr := tickets.Epic{PRNumber: 412, PRURL: "https://gh/412"}
	cases := []struct {
		name      string
		e         tickets.Epic
		state     wire.EpicListState
		delivery  string
		live      *prstate.Live
		wantWords string
		wantChip  string
		wantKinds []wire.CardActionKind
	}{
		{"pr project awaiting pr", tickets.Epic{}, wire.EpicListAwaitingPR, "pr", nil, "", "", []wire.CardActionKind{wire.CardActionCreatePR}},
		{"merge project awaiting merge", tickets.Epic{}, wire.EpicListAwaitingMerge, "merge", nil, "", "", []wire.CardActionKind{wire.CardActionCreatePR}},
		{"merge project, merge blocked: no create", tickets.Epic{}, wire.EpicListMergeBlocked, "merge", nil, "", "", nil},
		{"draft running", pr, wire.EpicListPROpen, "pr", &prstate.Live{Draft: true, Pending: 2, Green: 3}, "PR draft · CI running", "#412 · draft",
			[]wire.CardActionKind{wire.CardActionMarkMerged, wire.CardActionOpenPR}},
		{"ci failing: try again first, no create", pr, wire.EpicListCIFailing, "pr", nil, "CI failing", "#412",
			[]wire.CardActionKind{wire.CardActionTryAgain, wire.CardActionOpenEpic, wire.CardActionMarkMerged, wire.CardActionOpenPR}},
		{"pr text failed: try again", tickets.Epic{PRNumber: 5}, wire.EpicListPRTextFailed, "pr", nil, "", "#5",
			[]wire.CardActionKind{wire.CardActionTryAgain, wire.CardActionOpenEpic, wire.CardActionMarkMerged}},
		{"merge-blocked without a PR: no try again", tickets.Epic{}, wire.EpicListPaused, "pr", nil, "", "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := &Epics{Deps: Deps{
				Delivery: func(string) string { return tc.delivery },
				PRView: func(queue.RootRef) (prstate.Live, bool) {
					if tc.live == nil {
						return prstate.Live{}, false
					}
					return *tc.live, true
				},
				PRReason: func(queue.RootRef) string { return "gh is not logged in, run `gh auth login`" },
			}}
			row := wire.EpicListRow{State: tc.state, Actions: []wire.CardAction{}}
			if tc.state == wire.EpicListCIFailing || tc.state == wire.EpicListPRTextFailed {
				row.Actions = []wire.CardAction{open}
			}
			s.decoratePR(&row, root, tc.e)
			if tc.wantWords != "" && row.StatusWords != tc.wantWords {
				t.Errorf("words = %q, want %q", row.StatusWords, tc.wantWords)
			}
			if (tc.wantChip == "") != (row.PR == nil) || (row.PR != nil && row.PR.Chip != tc.wantChip) {
				t.Errorf("pr = %+v, want chip %q", row.PR, tc.wantChip)
			}
			if got := kinds(row.Actions); len(got) != len(tc.wantKinds) && !(tc.wantKinds == nil && len(row.Actions) == 0) {
				t.Errorf("actions = %v, want %v", got, tc.wantKinds)
			} else {
				for i, k := range tc.wantKinds {
					if got[i] != k {
						t.Errorf("actions = %v, want %v", got, tc.wantKinds)
						break
					}
				}
			}
			if row.PRReason == "" {
				t.Error("the failure reason is missing")
			}
		})
	}
}

func TestDecoratePR_CIFixAttemptCountsFromTryAgainAndShowsConfiguredCap(t *testing.T) {
	t.Parallel()
	reset := time.Unix(100, 0)
	old := tickets.Ticket{Identifier: "01", Type: string(schema.TypeCIFix), CreatedAt: reset.Add(-time.Hour)}
	fresh := tickets.Ticket{Identifier: "02", Type: string(schema.TypeCIFix), CreatedAt: reset.Add(time.Hour)}
	e := tickets.Epic{PRNumber: 412, Tickets: []tickets.Ticket{old, fresh}}
	root := queue.RootRef{Project: "proj", Epic: "epic"}
	s := &Epics{Deps: Deps{
		Delivery: func(string) string { return "pr" },
		PRView:   func(queue.RootRef) (prstate.Live, bool) { return prstate.Live{Draft: true, Failed: 1, Green: 1}, true },
		PRCIFix:  func(tickets.Epic) (time.Time, int) { return reset, 5 },
	}}
	row := wire.EpicListRow{State: wire.EpicListFixingCI}
	s.decoratePR(&row, root, e)
	if want := "Fixing CI (attempt 1 of 5)"; row.StatusWords != want {
		t.Errorf("row words = %q, want %q", row.StatusWords, want)
	}
	d := wire.EpicDetail{State: wire.EpicState(wire.EpicListFixingCI)}
	s.decoratePRDetail(&d, root, e)
	if want := "Fixing CI (attempt 1 of 5)"; d.StatusWords != want {
		t.Errorf("detail words = %q, want %q", d.StatusWords, want)
	}
}

func TestDecoratePR_NoCreatePROnAOneOffOrAMergedEpic(t *testing.T) {
	t.Parallel()
	if got := prActions(queue.RootRef{Project: "p", Epic: queue.OneOffsEpic}, tickets.Epic{}, wire.EpicListAwaitingPR, "pr", nil); got != nil {
		t.Errorf("one-off actions = %v", got)
	}
}
