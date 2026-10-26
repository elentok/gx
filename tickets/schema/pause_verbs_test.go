package schema

import (
	"slices"
	"strings"
	"testing"
)

func TestPauseRows_EveryKindHasARowWithAWebVerb(t *testing.T) {
	if err := checkPauseRows(PauseKinds(), pauseRows, pauseVerbs, graphEventsNotPauses); err != nil {
		t.Fatal(err)
	}
}

func TestPauseRows_GraphEventsNotPausesHoldsExactlyTwo(t *testing.T) {
	want := []PauseReason{PauseBlockedCycle, PauseParentDefect}
	if !slices.Equal(graphEventsNotPauses, want) {
		t.Fatalf("graphEventsNotPauses = %v, want %v", graphEventsNotPauses, want)
	}
	for _, r := range want {
		if _, ok := r.Row(); ok {
			t.Errorf("%s is exempt but has a row", r)
		}
	}
}

func TestCheckPauseRows_FailsOnMissingRowAndCLIOnlyRow(t *testing.T) {
	verbInfo := map[PauseVerb]VerbInfo{"web": {Web: true}, "cli": {Web: false}}
	kinds := []PauseReason{"fake-missing", "fake-cli", "fake-ok", "fake-exempt"}
	rows := map[PauseReason]PauseRow{
		"fake-cli": {Verbs: []PauseVerb{"cli"}},
		"fake-ok":  {Verbs: []PauseVerb{"cli", "web"}},
	}
	err := checkPauseRows(kinds, rows, verbInfo, []PauseReason{"fake-exempt"})
	if err == nil {
		t.Fatal("want an error")
	}
	for _, want := range []string{"fake-missing: no row", "fake-cli: no verb the web serves"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
	for _, bad := range []string{"fake-ok", "fake-exempt"} {
		if strings.Contains(err.Error(), bad) {
			t.Errorf("error %q names %q", err, bad)
		}
	}
}

func TestPauseRows_TableMatchesSpec(t *testing.T) {
	cases := []struct {
		reason PauseReason
		class  PauseClass
		who    PauseReceiver
		verbs  []PauseVerb
	}{
		{PauseSelfReported, PauseClassAnswer, ReceiverAgent, []PauseVerb{VerbAnswer, VerbResume}},
		{PauseManual, PauseClassAnswer, ReceiverGx, []PauseVerb{VerbResume, VerbStartOver}},
		{PauseDirtyTrunk, PauseClassAnswer, ReceiverGx, []PauseVerb{VerbStashMerge, VerbMergeAgain}},
		{PauseNeedsRebase, PauseClassRepair, "", []PauseVerb{VerbMergeAgain}},
		{PauseConflictMarkers, PauseClassRepair, "", []PauseVerb{VerbRetry, VerbStartOver}},
	}
	for _, c := range cases {
		row, ok := c.reason.Row()
		if !ok || row.Receiver != c.who || !slices.Equal(row.Verbs, c.verbs) {
			t.Errorf("%s: row = %+v, ok=%v", c.reason, row, ok)
		}
		if c.reason.Class() != c.class {
			t.Errorf("%s: class = %q, want %q", c.reason, c.reason.Class(), c.class)
		}
	}
}

func TestPausedAskingAndForRepair(t *testing.T) {
	cases := []struct {
		reason         PauseReason
		asking, repair bool
	}{
		{PauseSelfReported, true, false},
		{PauseManual, false, true},
		{PauseDirtyTrunk, false, true},
		{PauseIterationError, false, true},
	}
	for _, c := range cases {
		tk := Ticket{Status: StatusPaused, PauseReason: c.reason}
		if tk.PausedAsking() != c.asking || tk.PausedForRepair() != c.repair {
			t.Errorf("%s: asking=%v repair=%v", c.reason, tk.PausedAsking(), tk.PausedForRepair())
		}
	}
}

func TestPauseVerbs_EveryVerbHasLabelAndTooltip(t *testing.T) {
	for v, info := range pauseVerbs {
		if info.Label == "" || info.Tooltip == "" {
			t.Errorf("verb %q: label %q tooltip %q", v, info.Label, info.Tooltip)
		}
	}
	for r, row := range pauseRows {
		for _, v := range row.Verbs {
			if _, ok := pauseVerbs[v]; !ok {
				t.Errorf("%s lists unknown verb %q", r, v)
			}
		}
	}
}
