package schema

import "testing"

func validTicket() Ticket {
	return Ticket{
		ID:                    "04b",
		Status:                StatusReadyForAgent,
		BlockedBy:             []TicketID{"01", "03"},
		Type:                  TypeTask,
		CodeReviewFixes:       "none",
		ExpectedContextWindow: 20000,
		ActualContextWindow:   45230,
		ElapsedTime:           3612,
	}
}

func TestTicketID_Valid(t *testing.T) {
	tests := []struct {
		id   TicketID
		want bool
	}{
		{"04", true},
		{"06b", true},
		{"4", false},
		{"4bb", false},
		{"04-", false},
	}
	for _, tt := range tests {
		if got := tt.id.Valid(); got != tt.want {
			t.Errorf("TicketID(%q).Valid() = %v, want %v", tt.id, got, tt.want)
		}
	}
}

func TestStatus_Valid(t *testing.T) {
	canonical := []Status{
		StatusOpen, StatusNeedsTriage, StatusReadyForAgent, StatusReadyForHuman,
		StatusClaimed, StatusNeedsInfo, StatusNeedsAttention, StatusDone, StatusSuperseded,
	}
	if len(canonical) != 9 {
		t.Fatalf("expected exactly 9 canonical statuses, got %d", len(canonical))
	}
	for _, s := range canonical {
		if !s.Valid() {
			t.Errorf("Status(%q).Valid() = false, want true", s)
		}
	}
	if Status("error").Valid() {
		t.Errorf(`Status("error").Valid() = true, want false`)
	}
}

func TestValidate_Valid(t *testing.T) {
	if err := Validate(validTicket()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_BadID(t *testing.T) {
	tk := validTicket()
	tk.ID = "4"
	if err := Validate(tk); err == nil {
		t.Fatal("expected error for bad id, got nil")
	}
}

func TestValidate_BadStatus(t *testing.T) {
	tk := validTicket()
	tk.Status = "error"
	if err := Validate(tk); err == nil {
		t.Fatal("expected error for bad status, got nil")
	}
}

func TestValidate_BadType(t *testing.T) {
	tk := validTicket()
	tk.Type = "bogus"
	if err := Validate(tk); err == nil {
		t.Fatal("expected error for bad type, got nil")
	}
}

func TestValidate_BadCodeReviewFixes(t *testing.T) {
	tk := validTicket()
	tk.CodeReviewFixes = "maybe"
	if err := Validate(tk); err == nil {
		t.Fatal("expected error for bad code_review_fixes, got nil")
	}
}

func TestValidate_CodeReviewFixesTicketRef(t *testing.T) {
	tk := validTicket()
	tk.CodeReviewFixes = "ticket:05a"
	if err := Validate(tk); err != nil {
		t.Fatalf("unexpected error for valid ticket: ref: %v", err)
	}
}

func TestValidate_CodeReviewFixesEmptyIsUnset(t *testing.T) {
	tk := validTicket()
	tk.CodeReviewFixes = ""
	if err := Validate(tk); err != nil {
		t.Fatalf("unexpected error for unset code_review_fixes: %v", err)
	}
}

func TestValidate_NegativeExpectedContextWindow(t *testing.T) {
	tk := validTicket()
	tk.ExpectedContextWindow = -1
	if err := Validate(tk); err == nil {
		t.Fatal("expected error for negative expected_context_window, got nil")
	}
}

func TestValidate_NegativeActualContextWindow(t *testing.T) {
	tk := validTicket()
	tk.ActualContextWindow = -1
	if err := Validate(tk); err == nil {
		t.Fatal("expected error for negative actual_context_window, got nil")
	}
}

func TestValidate_NegativeElapsedTime(t *testing.T) {
	tk := validTicket()
	tk.ElapsedTime = -1
	if err := Validate(tk); err == nil {
		t.Fatal("expected error for negative elapsed_time, got nil")
	}
}

func TestValidate_SelfReferencingBlockedBy(t *testing.T) {
	tk := validTicket()
	tk.BlockedBy = []TicketID{tk.ID}
	if err := Validate(tk); err == nil {
		t.Fatal("expected error for self-referencing blocked_by, got nil")
	}
}

func TestValidate_SelfReferencingSplitFrom(t *testing.T) {
	tk := validTicket()
	self := tk.ID
	tk.SplitFrom = &self
	if err := Validate(tk); err == nil {
		t.Fatal("expected error for self-referencing split_from, got nil")
	}
}

func TestValidate_NonSelfReferencingSplitFromIsValid(t *testing.T) {
	tk := validTicket()
	parent := TicketID("04")
	tk.SplitFrom = &parent
	if err := Validate(tk); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
