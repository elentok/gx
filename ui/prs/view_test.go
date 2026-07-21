package prs

import (
	"strings"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
)

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
