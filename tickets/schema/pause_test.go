package schema

import (
	"strings"
	"testing"

	"github.com/elentok/gx/events"
)

func TestPauseClasses_MatchEventKinds(t *testing.T) {
	for r, class := range pauseClasses {
		k := events.Kind(r)
		if !k.Valid() {
			t.Errorf("reason %q is not an events kind", r)
			continue
		}
		if k.AwaitsAnswer() != (class == PauseClassAnswer) {
			t.Errorf("reason %q: events says AwaitsAnswer=%v, schema says class %q", r, k.AwaitsAnswer(), class)
		}
	}
}

func TestPauseReason_EveryReasonHasAClass(t *testing.T) {
	for r, class := range pauseClasses {
		if class != PauseClassAnswer && class != PauseClassRepair {
			t.Errorf("reason %q has class %q", r, class)
		}
		if !r.Valid() || r.Class() != class {
			t.Errorf("reason %q: Valid=%v Class=%q", r, r.Valid(), r.Class())
		}
		for _, old := range []string{"park", "land"} {
			if strings.Contains(string(r), old) {
				t.Errorf("reason %q keeps old word %q", r, old)
			}
		}
	}
	if PauseReason("nope").Valid() || PauseReason("nope").Class() != "" {
		t.Error("unknown reason must be invalid with no class")
	}
}

func TestPaused_RoundTripsWithReason(t *testing.T) {
	in := Ticket{ID: "01", Status: StatusPaused, Type: TypeImplement, PauseReason: PauseMergeConflict}
	out, err := MarshalTicket(in, "# T\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "pause_reason: merge-conflict") {
		t.Fatalf("pause_reason not written:\n%s", out)
	}
	got, err := ParseTicketFromRaw(string(out), "01.md")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusPaused || got.PauseReason != PauseMergeConflict {
		t.Fatalf("got %q/%q", got.Status, got.PauseReason)
	}
}

func TestPauseReasonText_IsDerivedAndNeverWritten(t *testing.T) {
	body := "# T\n\n## Paused\n\n**Merge** conflict in `a.go`\n\nmore\n"
	if got := PauseReasonText(body); got != "Merge conflict in a.go" {
		t.Errorf("PauseReasonText = %q", got)
	}
	if got := PauseReasonText("# T\n\n## Needs Repair\n\nold form\n"); got != "old form" {
		t.Errorf("old heading text = %q", got)
	}
	out, err := MarshalTicket(Ticket{ID: "01", Status: StatusPaused, Type: TypeImplement, PauseReason: PauseManual}, body)
	if err != nil {
		t.Fatal(err)
	}
	front, _, _ := SplitFrontmatter(string(out))
	if strings.Contains(front, "pause_reason_text") || strings.Contains(front, "Merge") {
		t.Errorf("derived text leaked into frontmatter:\n%s", front)
	}
}

func TestShimLegacyPause(t *testing.T) {
	cases := []struct {
		name   string
		status Status
		kind   PauseKind
		want   PauseReason
	}{
		{"answer with kind", StatusLegacyAnswer, "blocked-pane", PauseBlockedPane},
		{"answer without kind", StatusLegacyAnswer, "", PauseZeroCommit},
		{"repair with kind", StatusLegacyRepair, "budget-killed", PauseBudgetKilled},
		{"repair old spelling", StatusLegacyRepair, "land-conflict", PauseMergeConflict},
		{"repair without kind", StatusLegacyRepair, "", PauseIterationError},
		{"class mismatch", StatusLegacyRepair, "zero-commit", PauseIterationError},
		{"unknown kind", StatusLegacyAnswer, "bogus", PauseZeroCommit},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ShimLegacyPause(Ticket{ID: "01", Status: c.status, PauseKind: c.kind})
			if got.Status != StatusPaused || got.PauseReason != c.want || got.PauseKind != "" {
				t.Errorf("got %q/%q/%q, want paused/%q", got.Status, got.PauseReason, got.PauseKind, c.want)
			}
		})
	}
	open := Ticket{ID: "01", Status: StatusOpen}
	if got := ShimLegacyPause(open); got.Status != StatusOpen {
		t.Errorf("open ticket changed to %q", got.Status)
	}
}

func TestValidate_UnknownStatusStillRejected(t *testing.T) {
	err := Validate(Ticket{ID: "01", Status: "pausd", Type: TypeImplement})
	if err == nil || !strings.Contains(err.Error(), "invalid status") {
		t.Fatalf("err = %v", err)
	}
}
