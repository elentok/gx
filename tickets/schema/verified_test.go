package schema

import (
	"reflect"
	"testing"
	"time"
)

const oldBody = "# Title\n\n## What to build\n\nThing.\n\n## Comments\n\nhi\n"

func TestVerifiedRoundTrip(t *testing.T) {
	a := VerifiedEntry{Story: "45 launch proof", Verdict: "pass", Commit: "abc123"}
	b := VerifiedEntry{Story: "69 old body — parses", Verdict: "fail", Commit: "def456"}
	body := AppendVerified(AppendVerified(oldBody, a), b)

	if got := ParseVerified(body); !reflect.DeepEqual(got, []VerifiedEntry{a, b}) {
		t.Fatalf("ParseVerified = %+v", got)
	}
	if !a.Counts("abc123") || a.Counts("def456") || a.Counts("") {
		t.Fatal("Counts must match only the entry's own commit")
	}
	if got := Section(body, "Comments"); got != "hi" {
		t.Fatalf("other sections changed: %q", got)
	}
}

func TestIsOptionalSection(t *testing.T) {
	if !IsOptionalSection("Iteration summaries") {
		t.Error("Iteration summaries should be optional")
	}
	if IsOptionalSection("Spec deviations") {
		t.Error("Spec deviations should not be optional")
	}
}

func TestMarkVerifiedHistory(t *testing.T) {
	body := AppendVerified(oldBody, VerifiedEntry{Story: "s", Verdict: "pass", Commit: "abc"})
	got := ParseVerified(MarkVerifiedHistory(body))
	if len(got) != 1 || !got[0].History || got[0].Counts("abc") {
		t.Fatalf("entries = %+v", got)
	}
	if MarkVerifiedHistory(oldBody) != oldBody {
		t.Fatal("body without Verified must be unchanged")
	}
}

func TestIterationSummaryRoundTrip(t *testing.T) {
	a := IterationSummary{
		SessionID: "s1", Time: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), Label: "gx",
		Did: "built it", WentWrong: "nothing", Decisions: "kiss", Left: "none",
	}
	b := a
	b.SessionID, b.Label = "s2", "claude"
	body := AppendIterationSummary(AppendIterationSummary(oldBody, a), b)

	if got := ParseIterationSummaries(body); !reflect.DeepEqual(got, []IterationSummary{a, b}) {
		t.Fatalf("ParseIterationSummaries = %+v", got)
	}
}

func TestOldBodyParsesUnchanged(t *testing.T) {
	if ParseVerified(oldBody) != nil || ParseIterationSummaries(oldBody) != nil {
		t.Fatal("old body must yield no entries")
	}
}
