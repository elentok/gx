package schema

import "testing"

func TestStatus_DraftIsValid(t *testing.T) {
	if !StatusDraft.Valid() {
		t.Error("draft is not accepted as a status")
	}
	if err := Validate(Ticket{ID: "01", Status: StatusDraft, Type: TypeTask}); err != nil {
		t.Errorf("Validate() = %v, want nil for a draft ticket", err)
	}
}

func validTicket() Ticket {
	return Ticket{
		ID:                    "04b",
		Status:                StatusOpen,
		BlockedBy:             []TicketID{"01", "03"},
		Type:                  TypeTask,
		ExpectedContextWindow: 20000,
		ActualContextWindow:   45230,
		ElapsedTime:           3612,
	}
}

func TestValidate_CommitlessIsValidRegardlessOfValue(t *testing.T) {
	tk := validTicket()
	tk.Commitless = true
	if err := Validate(tk); err != nil {
		t.Fatalf("unexpected error for commitless=true: %v", err)
	}
}

func TestIsCommitless(t *testing.T) {
	tests := []struct {
		name string
		tk   Ticket
		want bool
	}{
		{"task, no flag", Ticket{Type: TypeTask}, false},
		{"task, flagged", Ticket{Type: TypeTask, Commitless: true}, true},
		{"grilling, no flag", Ticket{Type: TypeGrilling}, true},
		{"prototype, no flag", Ticket{Type: TypePrototype}, false},
		{"prototype, flagged", Ticket{Type: TypePrototype, Commitless: true}, true},
		{"research, no flag", Ticket{Type: TypeResearch}, true},
		{"code-review, no flag", Ticket{Type: TypeCodeReview}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tk.IsCommitless(); got != tt.want {
				t.Errorf("IsCommitless() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTicketID_Valid(t *testing.T) {
	tests := []struct {
		id   TicketID
		want bool
	}{
		{"04", true},
		{"06b", true},
		{"12b1", true},
		{"4", false},
		{"4bb", false},
		{"04-", false},
		{"12ab", false},
		{"12b1a", false},
	}
	for _, tt := range tests {
		if got := tt.id.Valid(); got != tt.want {
			t.Errorf("TicketID(%q).Valid() = %v, want %v", tt.id, got, tt.want)
		}
	}
}

func TestStatus_Valid(t *testing.T) {
	canonical := []Status{
		StatusDraft, StatusOpen, StatusClaimed,
		StatusNeedsAnswer, StatusNeedsRepair, StatusDone,
	}
	if len(canonical) != 6 {
		t.Fatalf("expected exactly 6 canonical statuses, got %d", len(canonical))
	}
	for _, s := range canonical {
		if !s.Valid() {
			t.Errorf("Status(%q).Valid() = false, want true", s)
		}
	}
	for _, s := range []Status{"error", "", "needs-triage", "ready-for-agent", "ready-for-human"} {
		if s.Valid() {
			t.Errorf("Status(%q).Valid() = true, want false", s)
		}
	}
}

func TestIterationStatus_Valid(t *testing.T) {
	for _, s := range []IterationStatus{"", IterationStatusWorking, IterationStatusNeedsAnswer, IterationStatusFinished} {
		if !s.Valid() {
			t.Errorf("IterationStatus(%q).Valid() = false, want true", s)
		}
	}
	for _, s := range []IterationStatus{"bogus", "Working", "needs-info"} {
		if s.Valid() {
			t.Errorf("IterationStatus(%q).Valid() = true, want false", s)
		}
	}
}

func TestValidate_Valid(t *testing.T) {
	if err := Validate(validTicket()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_DoesNotRejectUnrecognizedIterationStatus(t *testing.T) {
	tk := validTicket()
	tk.IterationStatus = "bogus"
	if err := Validate(tk); err != nil {
		t.Fatalf("unexpected error: %v, want no rejection (enum enforcement is write-conditional)", err)
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

func TestValidate_CodeReviewIsValidType(t *testing.T) {
	tk := validTicket()
	tk.Type = TypeCodeReview
	if err := Validate(tk); err != nil {
		t.Fatalf("unexpected error for type=code-review: %v", err)
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

func TestValidate_SelfReferencingParent(t *testing.T) {
	tk := validTicket()
	self := tk.ID
	tk.Parent = &self
	if err := Validate(tk); err == nil {
		t.Fatal("expected error for self-referencing parent, got nil")
	}
}

func TestValidate_NonSelfReferencingParentIsValid(t *testing.T) {
	tk := validTicket()
	parent := TicketID("04")
	tk.Parent = &parent
	if err := Validate(tk); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
