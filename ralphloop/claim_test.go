package ralphloop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/tickets"
)

func writeTicket(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "01-some-ticket.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return string(raw)
}

func TestClaim_RewritesExistingPlainStatusLine(t *testing.T) {
	path := writeTicket(t, "# Ticket\n\n**Blocked by:** None\n\nStatus: open\n\nSome body text.\n")
	if err := Claim(path); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	got := mustRead(t, path)
	want := "# Ticket\n\n**Blocked by:** None\n\nStatus: claimed\n\nSome body text.\n"
	if got != want {
		t.Errorf("Claim rewrote file as:\n%q\nwant:\n%q", got, want)
	}
}

func TestClaim_RewritesExistingBoldStatusLine(t *testing.T) {
	path := writeTicket(t, "# Ticket\n\n**Blocked by:** None\n\n**Status:** open\n\nSome body text.\n")
	if err := Claim(path); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	got := mustRead(t, path)
	want := "# Ticket\n\n**Blocked by:** None\n\n**Status:** claimed\n\nSome body text.\n"
	if got != want {
		t.Errorf("Claim rewrote file as:\n%q\nwant:\n%q", got, want)
	}
}

func TestClaim_PreservesRestOfFileByteForByte(t *testing.T) {
	original := "# Ticket\n\n**Type:** task\n\n**Blocked by:** 02, 03\n\n**Status:** open\n\n" +
		"- [ ] some criterion\n- [ ] another **bold** criterion\n\nTrailing prose with `code` and weird\tformatting.\n"
	path := writeTicket(t, original)
	if err := Claim(path); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	got := mustRead(t, path)
	want := "# Ticket\n\n**Type:** task\n\n**Blocked by:** 02, 03\n\n**Status:** claimed\n\n" +
		"- [ ] some criterion\n- [ ] another **bold** criterion\n\nTrailing prose with `code` and weird\tformatting.\n"
	if got != want {
		t.Errorf("Claim rewrote file as:\n%q\nwant:\n%q", got, want)
	}
}

func TestClaim_AddsStatusLineAfterLastMetadataLine(t *testing.T) {
	path := writeTicket(t, "# Ticket\n\n**Type:** task\n\n**Blocked by:** None\n\nSome body text.\n")
	if err := Claim(path); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	got := mustRead(t, path)
	want := "# Ticket\n\n**Type:** task\n\n**Blocked by:** None\n\n**Status:** claimed\n\nSome body text.\n"
	if got != want {
		t.Errorf("Claim rewrote file as:\n%q\nwant:\n%q", got, want)
	}
}

func TestClaim_AddsStatusLineWhenNoMetadataAtAll(t *testing.T) {
	path := writeTicket(t, "# Ticket\n\nJust a body, no metadata lines.\n")
	if err := Claim(path); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	got := mustRead(t, path)
	want := "# Ticket\n\nStatus: claimed\n\nJust a body, no metadata lines.\n"
	if got != want {
		t.Errorf("Claim rewrote file as:\n%q\nwant:\n%q", got, want)
	}
}

func TestMarkDone_RewritesStatusLine(t *testing.T) {
	path := writeTicket(t, "# Ticket\n\n**Status:** claimed\n\nBody.\n")
	if err := MarkDone(path); err != nil {
		t.Fatalf("MarkDone: %v", err)
	}
	got := mustRead(t, path)
	want := "# Ticket\n\n**Status:** done\n\nBody.\n"
	if got != want {
		t.Errorf("MarkDone rewrote file as:\n%q\nwant:\n%q", got, want)
	}
}

func TestClaimThenMarkDone_RoundTripsThroughParseTicket(t *testing.T) {
	path := writeTicket(t, "# Ticket\n\n**Blocked by:** None\n\nSome body.\n")

	if err := Claim(path); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	ticket, err := tickets.ParseTicket(mustRead(t, path))
	if err != nil {
		t.Fatalf("ParseTicket: %v", err)
	}
	if ticket.Status != "claimed" {
		t.Errorf("after Claim, Status = %q, want claimed", ticket.Status)
	}

	if err := MarkDone(path); err != nil {
		t.Fatalf("MarkDone: %v", err)
	}
	ticket, err = tickets.ParseTicket(mustRead(t, path))
	if err != nil {
		t.Fatalf("ParseTicket: %v", err)
	}
	if ticket.Status != "done" {
		t.Errorf("after MarkDone, Status = %q, want done", ticket.Status)
	}
	if !ticket.IsDone() {
		t.Errorf("after MarkDone, IsDone() = false, want true")
	}
}

func TestClaim_MissingFileReturnsError(t *testing.T) {
	if err := Claim(filepath.Join(t.TempDir(), "does-not-exist.md")); err == nil {
		t.Error("Claim(missing file) = nil error, want error")
	}
}
