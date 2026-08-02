package git

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseClosedPRList_Empty(t *testing.T) {
	prs, err := parseClosedPRList("[]")
	if err != nil {
		t.Fatalf("parseClosedPRList: %v", err)
	}
	if len(prs) != 0 {
		t.Fatalf("expected no PRs, got %d", len(prs))
	}
}

func TestParseClosedPRList_BlankOutput(t *testing.T) {
	prs, err := parseClosedPRList("")
	if err != nil {
		t.Fatalf("parseClosedPRList: %v", err)
	}
	if len(prs) != 0 {
		t.Fatalf("expected no PRs, got %d", len(prs))
	}
}

func TestParseClosedPRList_DecodesFields(t *testing.T) {
	out := `[{"number":7,"title":"Fix flake","state":"MERGED","mergedAt":"2026-07-19T10:00:00Z","closedAt":"2026-07-19T10:00:00Z","url":"https://github.com/o/r/pull/7"}]`
	prs, err := parseClosedPRList(out)
	if err != nil {
		t.Fatalf("parseClosedPRList: %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(prs))
	}
	pr := prs[0]
	if pr.Number != 7 || pr.Title != "Fix flake" || pr.URL != "https://github.com/o/r/pull/7" || !pr.IsMerged() {
		t.Fatalf("unexpected PR: %+v", pr)
	}
}

func TestParseClosedPRList_InvalidJSON(t *testing.T) {
	if _, err := parseClosedPRList("not json"); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestClosedPR_IsMerged(t *testing.T) {
	if !(ClosedPR{State: "MERGED"}).IsMerged() {
		t.Error("expected MERGED state to report IsMerged")
	}
	if (ClosedPR{State: "CLOSED"}).IsMerged() {
		t.Error("expected CLOSED state to not report IsMerged")
	}
}

func TestSortClosedPRs(t *testing.T) {
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	a := ClosedPR{Number: 1, ClosedAt: older}
	b := ClosedPR{Number: 2, ClosedAt: newer}

	prs := []ClosedPR{a, b}
	sortClosedPRs(prs)

	if prs[0].Number != 2 || prs[1].Number != 1 {
		t.Fatalf("expected most-recently-closed first, got %+v", prs)
	}
}

func TestClosedPRSearchNode_ToClosedPR(t *testing.T) {
	const body = `{
		"number": 7,
		"title": "Fix flake",
		"url": "https://github.com/o/r/pull/7",
		"state": "MERGED",
		"mergedAt": "2026-07-19T10:00:00Z",
		"closedAt": "2026-07-19T10:00:00Z",
		"repository": {"name": "r", "owner": {"login": "o"}}
	}`
	var node closedPRSearchNode
	if err := json.Unmarshal([]byte(body), &node); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	pr := node.toClosedPR()
	if pr.Number != 7 || pr.Title != "Fix flake" || pr.URL != "https://github.com/o/r/pull/7" || !pr.IsMerged() {
		t.Fatalf("unexpected closed PR: %+v", pr)
	}
	if pr.Repo != "o/r" {
		t.Fatalf("expected repo o/r, got %q", pr.Repo)
	}
}
