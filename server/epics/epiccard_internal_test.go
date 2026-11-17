package epics

import (
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func TestClaimedPhase_LatestPhaseWins(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ts   []tickets.Ticket
		want string
	}{
		{"no claimed ticket", []tickets.Ticket{typed("01", "open", schema.TypeVerify)}, "Implementing"},
		{"implement", []tickets.Ticket{typed("01", "claimed", schema.TypeImplement)}, "Implementing"},
		{"conflict resolution", []tickets.Ticket{typed("01", "claimed", schema.TypeConflictResolution)}, "Resolving conflicts"},
		{"trunk conflict", []tickets.Ticket{typed("01", "claimed", schema.TypeTrunkConflict)}, "Resolving conflicts"},
		{"conflicts beat implementing", []tickets.Ticket{
			typed("01", "claimed", schema.TypeImplement), typed("02", "claimed", schema.TypeTrunkConflict)}, "Resolving conflicts"},
		// Review and verify tickets wait for every other ticket, so they are
		// rendered claimed only once the rest is done.
		{"code review", []tickets.Ticket{
			typed("01", "done", schema.TypeImplement), typed("02", "claimed", schema.TypeCodeReview)}, "Code-reviewing"},
		{"verify", []tickets.Ticket{
			typed("01", "done", schema.TypeImplement), typed("02", "claimed", schema.TypeVerify)}, "Verifying"},
		{"ci fix reads implementing", []tickets.Ticket{typed("01", "claimed", schema.TypeCIFix)}, "Implementing"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := claimedPhase(tickets.Epic{Tickets: tc.ts}); got != tc.want {
				t.Errorf("phase = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDecorateLoopCard_WorkingFlagAndWords(t *testing.T) {
	t.Parallel()
	cases := []struct {
		state     wire.EpicListState
		phase     string
		working   bool
		word      string
		wantWords string
	}{
		{wire.EpicListRunning, "Verifying", true, "Verifying", "Verifying..."},
		{wire.EpicListRunning, "Implementing", true, "Implementing", "Implementing..."},
		{wire.EpicListMerging, "Implementing", true, "Merging", "Merging..."},
		{wire.EpicListWritingPRText, "Implementing", true, "Writing PR text", "Writing PR text"},
		{wire.EpicListFixingCI, "Implementing", true, "Fixing CI", "Not started"},
		{wire.EpicListPaused, "Verifying", false, "", "Implementing..."},
		{wire.EpicListQueued, "Implementing", false, "", "Queued #2"},
	}
	for _, tc := range cases {
		row := wire.EpicListRow{Address: "p:e", State: tc.state}
		decorateLoopCard(&row, 2, tc.phase)
		wantKind := wire.WorkKind("")
		if tc.working {
			wantKind = wire.WorkKindImplementation
		}
		if row.Working != tc.working || row.WorkingWord != tc.word || row.WorkKind != wantKind || row.StatusWords != tc.wantWords {
			t.Errorf("%s = working %v word %q kind %q words %q, want %v %q %q %q",
				tc.state, row.Working, row.WorkingWord, row.WorkKind, row.StatusWords, tc.working, tc.word, wantKind, tc.wantWords)
		}
	}
}

func TestJoinImplementation_BranchShowsDesignWhileImplIdle(t *testing.T) {
	t.Parallel()
	idle := wire.EpicListRow{Address: "p:m-impl", State: wire.EpicListReady, Section: wire.SectionReadyToQueue, StatusWords: "Not started"}
	working := wire.EpicListRow{Address: "p:m-impl", State: wire.EpicListRunning, Section: wire.SectionImplementing,
		StatusWords: "Implementing...", Working: true, WorkingWord: "Implementing", WorkKind: wire.WorkKindImplementation}
	cases := []struct {
		name     string
		step     wire.ProgressStep
		impl     wire.EpicListRow
		act      designActivity
		wantKind wire.WorkKind
		wantWord string
		wantWork bool
	}{
		{"design works at branch", wire.ProgressBranch, idle, designActivity{running: true, working: true}, wire.WorkKindDesign, "Branching", true},
		{"impl works", wire.ProgressBranch, working, designActivity{running: true, working: true}, wire.WorkKindImplementation, "Implementing", true},
		{"map done follows impl", wire.ProgressDone, working, designActivity{}, wire.WorkKindImplementation, "Implementing", true},
		{"design idle", wire.ProgressBranch, idle, designActivity{running: true}, "", "", false},
		{"countdown wins", wire.ProgressBranch, idle, designActivity{running: true, working: true, countdownAt: time.Now().Add(time.Minute)}, "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			row := wire.EpicListRow{Address: "p:m", Name: "m", ProgressStep: tc.step}
			impl := tc.impl
			joinImplementation(&row, &impl, tc.act)
			if row.Working != tc.wantWork || row.WorkingWord != tc.wantWord || row.WorkKind != tc.wantKind {
				t.Errorf("row = working %v word %q kind %q, want %v %q %q", row.Working, row.WorkingWord, row.WorkKind, tc.wantWork, tc.wantWord, tc.wantKind)
			}
			if tc.wantWork && tc.wantKind == wire.WorkKindDesign && row.StatusWords != "Branching..." {
				t.Errorf("words = %q, want Branching...", row.StatusWords)
			}
		})
	}
}

func TestJoinImplementation_QuestionStillWinsAtBranch(t *testing.T) {
	t.Parallel()
	impl := wire.EpicListRow{Address: "p:m-impl", State: wire.EpicListReady, Section: wire.SectionReadyToQueue}
	row := wire.EpicListRow{Address: "p:m", Name: "m", ProgressStep: wire.ProgressBranch}
	joinImplementation(&row, &impl, designActivity{running: true, asking: true, working: true})
	if row.Section != wire.SectionNeedsYou || row.Working {
		t.Errorf("row = section %q working %v, want Needs you and not working", row.Section, row.Working)
	}
}
