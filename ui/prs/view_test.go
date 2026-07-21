package prs

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

func TestRenderClosedRow_SelectedDiffersFromUnselected(t *testing.T) {
	m := Model{}
	pr := git.ClosedPR{Number: 5, Title: "Merged fix", State: "MERGED", ClosedAt: time.Now()}

	unselected := m.renderClosedRow(pr, false, 40)
	selected := m.renderClosedRow(pr, true, 40)

	if unselected == selected {
		t.Fatal("expected selected closed row to render differently than unselected")
	}
}

func TestRenderCIFacet_Labels(t *testing.T) {
	icons := ui.Icons(false)
	cases := []struct {
		state git.CIState
		want  string
	}{
		{git.CIRunning, "checking"},
		{git.CIFailed, "failing"},
		{git.CIPassed, "passing"},
	}
	for _, c := range cases {
		got := renderCIFacet(icons, c.state)
		if !strings.Contains(got, c.want) {
			t.Errorf("renderCIFacet(%v) = %q, want label %q", c.state, got, c.want)
		}
	}
}

func TestRenderCIFacet_NoneStaysSilent(t *testing.T) {
	icons := ui.Icons(false)
	got := renderCIFacet(icons, git.CINone)
	if strings.Contains(got, "checking") || strings.Contains(got, "failing") || strings.Contains(got, "passing") {
		t.Errorf("renderCIFacet(CINone) = %q, want no label", got)
	}
}

func TestRenderApprovalFacet_Labels(t *testing.T) {
	icons := ui.Icons(false)
	cases := []struct {
		state git.ApprovalState
		want  string
	}{
		{git.ApprovalApproved, "approved"},
		{git.ApprovalChangesRequested, "changes requested"},
		{git.ApprovalCommentedOnly, "commented"},
		{git.ApprovalNotYet, "review needed"},
	}
	for _, c := range cases {
		got := renderApprovalFacet(icons, c.state)
		if !strings.Contains(got, c.want) {
			t.Errorf("renderApprovalFacet(%v) = %q, want label %q", c.state, got, c.want)
		}
	}
}

func TestRenderMergeableFacet_Labels(t *testing.T) {
	icons := ui.Icons(false)
	cases := []struct {
		state git.MergeableState
		want  string
	}{
		{git.MergeableConflicting, "conflicts"},
		{git.MergeableChecking, "checking"},
	}
	for _, c := range cases {
		got := renderMergeableFacet(icons, c.state)
		if !strings.Contains(got, c.want) {
			t.Errorf("renderMergeableFacet(%v) = %q, want label %q", c.state, got, c.want)
		}
	}
}

func TestRenderMergeableFacet_CleanStaysSilent(t *testing.T) {
	icons := ui.Icons(false)
	if got := renderMergeableFacet(icons, git.MergeableClean); got != "" {
		t.Errorf("renderMergeableFacet(MergeableClean) = %q, want empty", got)
	}
}

func TestRenderCommentFacet_UnchangedNumericFormat(t *testing.T) {
	icons := ui.Icons(false)
	got := renderCommentFacet(icons, false, 3)
	if !strings.Contains(got, "3c") {
		t.Errorf("renderCommentFacet(3) = %q, want %q", got, "3c")
	}
	if got := renderCommentFacet(icons, false, 0); got != "" {
		t.Errorf("renderCommentFacet(0) = %q, want empty", got)
	}
}

// TestCombinedContentRangesMatchRenderedHighlight guards the model.go /
// view.go coupling from issues/01-dedupe-prs-layout-model.md: combinedContent
// is the single place that both renders lines and reports each item's line
// span, so model.go's scroll math can never drift from what actually gets
// rendered — including if a future change to renderRow/renderClosedRow grows
// or shrinks a row's line count. Selecting item i only changes the styling of
// the lines within its own range, so diffing the rendered output between two
// selections pins down exactly which lines "belong" to which item, and that
// must equal the ranges combinedContent reports.
func TestCombinedContentRangesMatchRenderedHighlight(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = loadPRs(m, []git.PR{
		greenPR(1, "Ready one"),
		greenPR(2, "Ready two"),
		neutralPR(3, "Waiting one"),
	}, true, nil, []git.ClosedPR{
		{Number: 4, Title: "Closed one", State: "MERGED", ClosedAt: time.Now()},
		{Number: 5, Title: "Closed two", State: "MERGED", ClosedAt: time.Now()},
	})

	linesFor := func(sel int) []string {
		m.list.SetSelected(sel, m.totalItems())
		lines, _ := m.combinedContent()
		return lines
	}

	total := m.totalItems()
	_, ranges := m.combinedContent()
	if len(ranges) != total {
		t.Fatalf("expected %d item ranges, got %d", total, len(ranges))
	}

	for i := 0; i < total-1; i++ {
		curr := linesFor(i)
		next := linesFor(i + 1)
		if len(curr) != len(next) {
			t.Fatalf("selecting different items changed total line count: %d vs %d", len(curr), len(next))
		}

		changed := map[int]bool{}
		for line := range curr {
			if curr[line] != next[line] {
				changed[line] = true
			}
		}

		want := map[int]bool{}
		for line := ranges[i].start; line < ranges[i].end; line++ {
			want[line] = true
		}
		for line := ranges[i+1].start; line < ranges[i+1].end; line++ {
			want[line] = true
		}

		if len(changed) != len(want) {
			t.Fatalf("item %d->%d: changed lines %v, want %v", i, i+1, changed, want)
		}
		for line := range want {
			if !changed[line] {
				t.Fatalf("item %d->%d: expected line %d to change (in reported range), it didn't", i, i+1, line)
			}
		}
	}
}
