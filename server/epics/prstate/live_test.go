package prstate

import (
	"testing"

	"github.com/elentok/gx/server/wire"
)

func TestInfoAndWords(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		merged     bool
		live       *Live
		state      wire.EpicListState
		wantChip   string
		wantChecks string
		wantWords  string
	}{
		{"restart: no live view", false, nil, wire.EpicListPROpen, "#412", "", "PR open"},
		{"draft running", false, &Live{Draft: true, Pending: 2, Green: 3}, wire.EpicListPROpen, "#412 · draft", "2 of 5 checks running", "PR draft · CI running"},
		{"ready green", false, &Live{Green: 5}, wire.EpicListPROpen, "#412 · ready", "5 of 5 checks passed", "Ready for review"},
		{"failed check", false, &Live{Draft: true, Failed: 1, Green: 4, FailedName: "go-test"}, wire.EpicListCIFailing, "#412 · draft", "go-test failed", "CI failing"},
		{"merged, merge blocked", false, &Live{Merged: true, Green: 5}, wire.EpicListMergeBlocked, "#412 · merged", "5 of 5 checks passed", "PR merged"},
		{"merged by stamp, no live", true, nil, wire.EpicListMergeBlocked, "#412 · merged", "", "PR merged"},
		{"fixing ci", false, &Live{Draft: true, Failed: 1, Green: 1}, wire.EpicListFixingCI, "#412 · draft", "1 of 2 checks failed", "Fixing CI (attempt 2 of 3)"},
		{"writing pr text", false, nil, wire.EpicListWritingPRText, "#412", "", "Writing PR text"},
		{"merge-blocked without a merged PR is not ours", false, &Live{Draft: true}, wire.EpicListMergeBlocked, "#412 · draft", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			info := Info(412, "https://x/412", tc.merged, tc.live)
			if info.Chip != tc.wantChip || info.Checks != tc.wantChecks {
				t.Errorf("chip, checks = %q, %q; want %q, %q", info.Chip, info.Checks, tc.wantChip, tc.wantChecks)
			}
			if got := Words(tc.state, &info, tc.live, 2, 3); got != tc.wantWords {
				t.Errorf("words = %q, want %q", got, tc.wantWords)
			}
		})
	}
}

func TestInfo_UnreachableRidesTheLiveView(t *testing.T) {
	t.Parallel()
	if info := Info(1, "", false, &Live{Unreachable: true}); !info.Unreachable {
		t.Error("Unreachable not carried")
	}
}
