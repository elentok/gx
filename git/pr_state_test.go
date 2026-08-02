package git

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCIState(t *testing.T) {
	cases := []struct {
		name  string
		pr    PR
		state CIState
	}{
		{"no checks", PR{}, CINone},
		{"all passed", PR{StatusCheckRollup: []PRStatusCheck{
			{Status: "COMPLETED", Conclusion: "SUCCESS"},
		}}, CIPassed},
		{"still running", PR{StatusCheckRollup: []PRStatusCheck{
			{Status: "COMPLETED", Conclusion: "SUCCESS"},
			{Status: "IN_PROGRESS"},
		}}, CIRunning},
		{"failed wins over running", PR{StatusCheckRollup: []PRStatusCheck{
			{Status: "IN_PROGRESS"},
			{Status: "COMPLETED", Conclusion: "FAILURE"},
		}}, CIFailed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.pr.CIState(); got != c.state {
				t.Errorf("expected %v, got %v", c.state, got)
			}
		})
	}
}

func TestApprovalState(t *testing.T) {
	cases := []struct {
		name      string
		pr        PR
		state     ApprovalState
		reviewers bool
	}{
		{"approved", PR{ReviewDecision: "APPROVED"}, ApprovalApproved, false},
		{"changes requested", PR{ReviewDecision: "CHANGES_REQUESTED"}, ApprovalChangesRequested, false},
		{"commented only", PR{Reviews: []PRReview{{State: "COMMENTED"}}}, ApprovalCommentedOnly, false},
		{"not yet, reviewers requested", PR{ReviewRequests: []json.RawMessage{[]byte(`{}`)}}, ApprovalNotYet, true},
		{"not yet, no reviewers", PR{}, ApprovalNotYet, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			state, reviewers := c.pr.ApprovalState()
			if state != c.state || reviewers != c.reviewers {
				t.Errorf("expected (%v, %v), got (%v, %v)", c.state, c.reviewers, state, reviewers)
			}
		})
	}
}

func TestMergeableState(t *testing.T) {
	cases := []struct {
		mergeable string
		state     MergeableState
	}{
		{"CONFLICTING", MergeableConflicting},
		{"MERGEABLE", MergeableClean},
		{"UNKNOWN", MergeableChecking},
		{"", MergeableChecking},
	}
	for _, c := range cases {
		pr := PR{Mergeable: c.mergeable}
		if got := pr.MergeableState(); got != c.state {
			t.Errorf("mergeable=%q: expected %v, got %v", c.mergeable, c.state, got)
		}
	}
}

func TestCommentCount(t *testing.T) {
	pr := PR{
		Comments: []json.RawMessage{[]byte(`{}`), []byte(`{}`)},
		Reviews: []PRReview{
			{Body: "looks good"},
			{Body: "   "},
			{Body: ""},
		},
	}
	if got := pr.CommentCount(); got != 3 {
		t.Errorf("expected 3, got %d", got)
	}
}

func TestSortPRs(t *testing.T) {
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	green := PR{
		Number:            1,
		StatusCheckRollup: []PRStatusCheck{{Status: "COMPLETED", Conclusion: "SUCCESS"}},
		ReviewDecision:    "APPROVED",
		Mergeable:         "MERGEABLE",
		UpdatedAt:         older,
	}
	red := PR{Number: 2, UpdatedAt: newer} // no reviewers requested → red
	neutralOld := PR{Number: 3, ReviewRequests: []json.RawMessage{[]byte(`{}`)}, UpdatedAt: older}
	neutralNew := PR{Number: 4, ReviewRequests: []json.RawMessage{[]byte(`{}`)}, UpdatedAt: newer}

	prs := []PR{neutralOld, red, neutralNew, green}
	sortPRs(prs)

	got := []int{prs[0].Number, prs[1].Number, prs[2].Number, prs[3].Number}
	want := []int{1, 2, 4, 3}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected order %v, got %v", want, got)
		}
	}
}

func TestMarker(t *testing.T) {
	cases := []struct {
		name   string
		pr     PR
		marker Marker
	}{
		{
			"green: passed + approved + clean",
			PR{
				StatusCheckRollup: []PRStatusCheck{{Status: "COMPLETED", Conclusion: "SUCCESS"}},
				ReviewDecision:    "APPROVED",
				Mergeable:         "MERGEABLE",
			},
			MarkerGreen,
		},
		{
			"red: CI failed",
			PR{StatusCheckRollup: []PRStatusCheck{{Status: "COMPLETED", Conclusion: "FAILURE"}}},
			MarkerRed,
		},
		{
			"red: changes requested",
			PR{ReviewDecision: "CHANGES_REQUESTED"},
			MarkerRed,
		},
		{
			"red: conflicting",
			PR{Mergeable: "CONFLICTING"},
			MarkerRed,
		},
		{
			"red: no reviewers requested",
			PR{},
			MarkerRed,
		},
		{
			"neutral: CI running",
			PR{
				StatusCheckRollup: []PRStatusCheck{{Status: "IN_PROGRESS"}},
				ReviewRequests:    []json.RawMessage{[]byte(`{}`)},
			},
			MarkerNeutral,
		},
		{
			"neutral: waiting on reviewers",
			PR{ReviewRequests: []json.RawMessage{[]byte(`{}`)}},
			MarkerNeutral,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.pr.Marker(); got != c.marker {
				t.Errorf("expected %v, got %v", c.marker, got)
			}
		})
	}
}
