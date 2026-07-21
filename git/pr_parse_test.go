package git

import "testing"

func TestParsePRList_Empty(t *testing.T) {
	prs, err := parsePRList("[]")
	if err != nil {
		t.Fatalf("parsePRList: %v", err)
	}
	if len(prs) != 0 {
		t.Fatalf("expected no PRs, got %d", len(prs))
	}
}

func TestParsePRList_BlankOutput(t *testing.T) {
	prs, err := parsePRList("")
	if err != nil {
		t.Fatalf("parsePRList: %v", err)
	}
	if len(prs) != 0 {
		t.Fatalf("expected no PRs, got %d", len(prs))
	}
}

func TestParsePRList_DecodesFields(t *testing.T) {
	out := `[{"number":42,"title":"Fix the thing","url":"https://github.com/o/r/pull/42","isDraft":true,"updatedAt":"2026-07-20T10:00:00Z"}]`
	prs, err := parsePRList(out)
	if err != nil {
		t.Fatalf("parsePRList: %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(prs))
	}
	pr := prs[0]
	if pr.Number != 42 || pr.Title != "Fix the thing" || pr.URL != "https://github.com/o/r/pull/42" || !pr.IsDraft {
		t.Fatalf("unexpected PR: %+v", pr)
	}
}

func TestParsePRList_InvalidJSON(t *testing.T) {
	if _, err := parsePRList("not json"); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
