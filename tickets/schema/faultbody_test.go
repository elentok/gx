package schema

import (
	"os"
	"strings"
	"testing"
)

func TestSplitReason_SingleLine_NoDetail(t *testing.T) {
	summary, detail := SplitReason("agent stalled")
	if summary != "agent stalled" {
		t.Errorf("summary = %q, want %q", summary, "agent stalled")
	}
	if detail != "" {
		t.Errorf("detail = %q, want empty", detail)
	}
}

func TestSplitReason_MultiLine_SummaryPlusDetail(t *testing.T) {
	summary, detail := SplitReason("agent stalled\n\nfull stack trace here\nmore lines")
	if summary != "agent stalled" {
		t.Errorf("summary = %q, want %q", summary, "agent stalled")
	}
	if detail != "full stack trace here\nmore lines" {
		t.Errorf("detail = %q, want %q", detail, "full stack trace here\nmore lines")
	}
}

func TestFormatFaultBody_EmptyReason_Rejected(t *testing.T) {
	if _, err := FormatFaultBody("", FaultState{}); err == nil {
		t.Fatal("expected error for empty reason, got nil")
	}
	if _, err := FormatFaultBody("   ", FaultState{}); err == nil {
		t.Fatal("expected error for whitespace-only reason, got nil")
	}
}

func TestFormatFaultBody_SingleLineReason_NoDetailSection(t *testing.T) {
	body, err := FormatFaultBody("agent stalled", FaultState{})
	if err != nil {
		t.Fatalf("FormatFaultBody: %v", err)
	}
	if !strings.Contains(body, PausedHeading) {
		t.Errorf("body = %q, want a %s heading", body, PausedHeading)
	}
	if !strings.Contains(body, "agent stalled") {
		t.Errorf("body = %q, want the summary line", body)
	}
}

func TestFormatFaultBody_MultiLineReason_SummaryPlusDetail(t *testing.T) {
	body, err := FormatFaultBody("agent stalled\n\nfull stack trace", FaultState{})
	if err != nil {
		t.Fatalf("FormatFaultBody: %v", err)
	}
	if !strings.Contains(body, "agent stalled") || !strings.Contains(body, "full stack trace") {
		t.Errorf("body = %q, want both summary and detail", body)
	}
}

func TestFormatFaultBody_StateOmitsUnavailableFields(t *testing.T) {
	body, err := FormatFaultBody("agent stalled", FaultState{Branch: "epic/04"})
	if err != nil {
		t.Fatalf("FormatFaultBody: %v", err)
	}
	if !strings.Contains(body, "epic/04") {
		t.Errorf("body = %q, want the branch present", body)
	}
	if strings.Contains(body, "Iteration:") || strings.Contains(body, "Worktree:") {
		t.Errorf("body = %q, want no placeholder for unavailable fields", body)
	}
}

func TestFormatFaultBody_NoHandoffSection(t *testing.T) {
	body, err := FormatFaultBody("agent stalled", FaultState{
		Label: "epic-04", Branch: "epic/04", Worktree: "/tmp/wt/epic-04",
	})
	if err != nil {
		t.Fatalf("FormatFaultBody: %v", err)
	}
	if strings.Contains(body, "## Handoff") {
		t.Errorf("body = %q, want no Handoff section from a fault write", body)
	}
}

func TestFaultWrite_RoundTripThroughUpdateTicket(t *testing.T) {
	path := writeTemp(t, "04b-ticket.md", "---\nid: \"04b\"\nstatus: claimed\ntype: implement\n---\nBody.\n")

	section, err := FormatFaultBody("agent stalled\n\nfull trace", FaultState{
		Label: "epic-04b", Branch: "epic/04b", Worktree: "/tmp/wt/epic-04b",
	})
	if err != nil {
		t.Fatalf("FormatFaultBody: %v", err)
	}

	err = UpdateTicket(path, func(tk *Ticket) {
		tk.Status = StatusLegacyRepair
	})
	if err != nil {
		t.Fatalf("UpdateTicket: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	// UpdateTicket leaves the body untouched; the section append is a
	// separate concern (the ralphloop fault path composes both).
	// Simulate that composition here to exercise the full round trip this
	// ticket's test seam calls for: a fault write produces the documented
	// shape on disk.
	if err := os.WriteFile(path, append(raw, []byte(section)...), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := ParseTicket(path)
	if err != nil {
		t.Fatalf("ParseTicket: %v (loader must accept the written shape)", err)
	}
	if got.Status != StatusPaused || got.PausedClass() != PauseClassRepair {
		t.Errorf("Status = %q reason %q, want paused with a repair reason", got.Status, got.PauseReason)
	}

	final, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	body := string(final)
	for _, want := range []string{PausedHeading, "agent stalled", "full trace", "epic-04b", "epic/04b", "/tmp/wt/epic-04b"} {
		if !strings.Contains(body, want) {
			t.Errorf("ticket body = %q, want it to contain %q", body, want)
		}
	}
	if strings.Contains(body, "## Handoff") {
		t.Errorf("ticket body = %q, want no Handoff section", body)
	}
}

func TestParseTicket_HandAuthoredFileWithoutPausedSection_StillLoads(t *testing.T) {
	path := writeTemp(t, "05-ticket.md", "---\nid: \"05\"\nstatus: needs-repair\ntype: implement\n---\nNo section here, just a note in prose.\n")

	if _, err := ParseTicket(path); err != nil {
		t.Errorf("ParseTicket: %v, want a hand-authored needs-repair ticket without the section to still load (validation is write-conditional, not at rest)", err)
	}
}
