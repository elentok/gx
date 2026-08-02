package schema

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}
	return path
}

const newFormatFixture = `---
id: "04b"
status: ready-for-agent
blocked_by: ["01", "03"]
type: task
code_review_fixes: none
expected_context_window: 20000
actual_context_window: 45230
elapsed_time: 3612
---
# 04b — Ticket title

**What to build:** ...
`

func TestParseTicket_NewFormat(t *testing.T) {
	path := writeTemp(t, "04b-ticket.md", newFormatFixture)

	got, err := ParseTicket(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := Ticket{
		ID:                    "04b",
		Status:                StatusReadyForAgent,
		BlockedBy:             []TicketID{"01", "03"},
		Type:                  TypeTask,
		CodeReviewFixes:       "none",
		ExpectedContextWindow: 20000,
		ActualContextWindow:   45230,
		ElapsedTime:           3612,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseTicket() = %+v, want %+v", got, want)
	}
}

func TestParseTicket_RoundTrip(t *testing.T) {
	path := writeTemp(t, "04b-ticket.md", newFormatFixture)

	t1, err := ParseTicket(path)
	if err != nil {
		t.Fatalf("initial parse: %v", err)
	}

	marshaled, err := MarshalTicket(t1, "# 04b — Ticket title\n\nBody text.\n")
	if err != nil {
		t.Fatalf("MarshalTicket: %v", err)
	}

	roundTripPath := writeTemp(t, "04b-roundtrip.md", string(marshaled))
	t2, err := ParseTicket(roundTripPath)
	if err != nil {
		t.Fatalf("re-parse after marshal: %v", err)
	}

	if !reflect.DeepEqual(t1, t2) {
		t.Fatalf("round trip mismatch: parsed %+v, got back %+v", t1, t2)
	}
}

func TestParseTicket_RoundTrip_SplitFrom(t *testing.T) {
	orig := Ticket{
		ID:              "04b",
		Status:          StatusClaimed,
		Type:            TypeTask,
		CodeReviewFixes: "",
	}
	parent := TicketID("04")
	orig.SplitFrom = &parent
	orig.Split = []TicketID{"04c", "04d"}

	marshaled, err := MarshalTicket(orig, "body\n")
	if err != nil {
		t.Fatalf("MarshalTicket: %v", err)
	}

	path := writeTemp(t, "04b-split.md", string(marshaled))
	got, err := ParseTicket(path)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if !reflect.DeepEqual(orig, got) {
		t.Fatalf("round trip mismatch: original %+v, got %+v", orig, got)
	}
}

func TestParseTicket_MalformedFrontmatterYAML(t *testing.T) {
	content := `---
id: "04b"
status: ready-for-agent
blocked_by: [01, 03
---
body
`
	path := writeTemp(t, "04b-bad.md", content)

	_, err := ParseTicket(path)
	if err == nil {
		t.Fatal("expected error for malformed frontmatter YAML, got nil")
	}
	if !strings.Contains(err.Error(), "parsing frontmatter") {
		t.Errorf("error %q does not mention frontmatter parsing", err.Error())
	}
}

func TestParseTicket_FrontmatterValidateErrorSurfaced(t *testing.T) {
	content := `---
id: "04b"
status: bogus-status
type: task
---
body
`
	path := writeTemp(t, "04b-invalid.md", content)

	_, err := ParseTicket(path)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "status") {
		t.Errorf("error %q does not mention the invalid status field", err.Error())
	}
}

func TestParseTicket_OldFormatFallback_QuitWarning(t *testing.T) {
	got, err := ParseTicket("testdata/03-quit-warning.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.ID != "03" {
		t.Errorf("ID = %q, want %q", got.ID, "03")
	}
	if got.Status != StatusDone {
		t.Errorf("Status = %q, want %q", got.Status, StatusDone)
	}
	if !reflect.DeepEqual(got.BlockedBy, []TicketID{"01"}) {
		t.Errorf("BlockedBy = %v, want [01]", got.BlockedBy)
	}
	if got.ActualContextWindow != 135265 {
		t.Errorf("ActualContextWindow = %d, want 135265", got.ActualContextWindow)
	}
	if got.CodeReviewFixes != "inline" {
		t.Errorf("CodeReviewFixes = %q, want %q", got.CodeReviewFixes, "inline")
	}
}

func TestParseTicket_OldFormatFallback_PersistentStatusOverlay(t *testing.T) {
	got, err := ParseTicket("testdata/06-persistent-status-overlay.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.ID != "06" {
		t.Errorf("ID = %q, want %q", got.ID, "06")
	}
	if got.Status != StatusDone {
		t.Errorf("Status = %q, want %q", got.Status, StatusDone)
	}
	if !reflect.DeepEqual(got.BlockedBy, []TicketID{"01"}) {
		t.Errorf("BlockedBy = %v, want [01]", got.BlockedBy)
	}
	if !reflect.DeepEqual(got.Split, []TicketID{"06b"}) {
		t.Errorf("Split = %v, want [06b]", got.Split)
	}
	if got.ActualContextWindow != 61905 {
		t.Errorf("ActualContextWindow = %d, want 61905", got.ActualContextWindow)
	}
	if got.CodeReviewFixes != "none" {
		t.Errorf("CodeReviewFixes = %q, want %q", got.CodeReviewFixes, "none")
	}
}

func TestParseTicket_OldFormatFallback_FollowingUpMapsToSplitFrom(t *testing.T) {
	content := `**Status:** open
**Blocked by:** 01
Following-up: 04

Body prose.
`
	path := writeTemp(t, "04b-followup.md", content)

	got, err := ParseTicket(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.SplitFrom == nil || *got.SplitFrom != "04" {
		t.Errorf("SplitFrom = %v, want pointer to 04", got.SplitFrom)
	}
}

func TestParseTicket_OldFormatFallback_NoFrontmatterDoesNotRunValidate(t *testing.T) {
	// Missing Status: (old format's valid open/unclaimed default) would fail
	// the new Status enum's Validate, but the old-format fallback must not
	// error on it.
	content := "# A ticket with no metadata lines\n\nJust prose.\n"
	path := writeTemp(t, "no-metadata.md", content)

	got, err := ParseTicket(path)
	if err != nil {
		t.Fatalf("unexpected error for old-format file with no Status:: %v", err)
	}
	if got.Status != "" {
		t.Errorf("Status = %q, want empty (no Status: line)", got.Status)
	}
}
