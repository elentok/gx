package ralphloop

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
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

func TestMarkDoneWithMetadata_InsertsContextWindowAndSessionAfterStatus(t *testing.T) {
	path := writeTicket(t, "# Ticket\n\n**Status:** claimed\n\nBody.\n")
	if err := MarkDoneWithMetadata(path, 42000, "sess-123"); err != nil {
		t.Fatalf("MarkDoneWithMetadata: %v", err)
	}
	got := mustRead(t, path)
	want := "# Ticket\n\n**Status:** done\n**Context window:** 42000\n**Session:** sess-123\n\nBody.\n"
	if got != want {
		t.Errorf("MarkDoneWithMetadata rewrote file as:\n%q\nwant:\n%q", got, want)
	}
}

func TestMarkDoneWithMetadata_MatchesPlainStatusStyle(t *testing.T) {
	path := writeTicket(t, "# Ticket\n\nStatus: claimed\n\nBody.\n")
	if err := MarkDoneWithMetadata(path, 1000, "sess-1"); err != nil {
		t.Fatalf("MarkDoneWithMetadata: %v", err)
	}
	got := mustRead(t, path)
	want := "# Ticket\n\nStatus: done\nContext window: 1000\nSession: sess-1\n\nBody.\n"
	if got != want {
		t.Errorf("MarkDoneWithMetadata rewrote file as:\n%q\nwant:\n%q", got, want)
	}
}

func TestMarkDoneWithMetadata_RoundTripsThroughParseTicket(t *testing.T) {
	path := writeTicket(t, "# Ticket\n\n**Status:** claimed\n\nBody.\n")
	if err := MarkDoneWithMetadata(path, 7, "sess-x"); err != nil {
		t.Fatalf("MarkDoneWithMetadata: %v", err)
	}
	ticket, err := tickets.ParseTicket(mustRead(t, path))
	if err != nil {
		t.Fatalf("ParseTicket: %v", err)
	}
	if ticket.Status != "done" {
		t.Errorf("Status = %q, want done", ticket.Status)
	}
	if !strings.Contains(ticket.Body, "Context window:** 7") || !strings.Contains(ticket.Body, "Session:** sess-x") {
		t.Errorf("Body = %q, want it to still contain the new metadata lines (unparsed, unaffected)", ticket.Body)
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

// TestClaim_FrontmatterTicket_RoundTripsThroughSchema verifies ticket 07:
// Claim on a frontmatter-format ticket rewrites status via
// schema.ParseTicketFromRaw/MarshalTicket rather than line-splicing, so the
// YAML block stays valid (rather than gaining a stray capitalized "Status:"
// line that no longer matches the "status" key).
func TestClaim_FrontmatterTicket_RoundTripsThroughSchema(t *testing.T) {
	path := writeTicket(t, "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# Ticket\n\nBody.\n")
	if err := Claim(path); err != nil {
		t.Fatalf("Claim: %v", err)
	}

	raw := mustRead(t, path)
	ticket, err := schema.ParseTicketFromRaw(raw, path)
	if err != nil {
		t.Fatalf("ParseTicketFromRaw: %v (raw=%q)", err, raw)
	}
	if ticket.Status != schema.StatusClaimed {
		t.Errorf("Status = %q, want claimed", ticket.Status)
	}
	if !strings.Contains(raw, "Body.") {
		t.Errorf("body content lost:\n%s", raw)
	}
}

// TestMarkDoneWithMetadata_FrontmatterTicket_WritesActualContextWindow
// verifies ticket 07's fix for the corruption described in its issue: a
// frontmatter ticket already carrying actual_context_window/elapsed_time
// (as ticket 05a's landCherryPick writes) stays valid and gx-tickets-validate
// -passing after MarkDoneWithMetadata, with status: done and
// actual_context_window updated in place — not spliced as stray Context
// window:/Session: lines inside the YAML block.
func TestMarkDoneWithMetadata_FrontmatterTicket_WritesActualContextWindow(t *testing.T) {
	path := writeTicket(t, "---\nid: \"01\"\nstatus: claimed\ntype: task\nactual_context_window: 500\nelapsed_time: 10\n---\n# Ticket\n\nBody.\n")
	if err := MarkDoneWithMetadata(path, 42000, "sess-123"); err != nil {
		t.Fatalf("MarkDoneWithMetadata: %v", err)
	}

	raw := mustRead(t, path)
	ticket, err := schema.ParseTicketFromRaw(raw, path)
	if err != nil {
		t.Fatalf("ParseTicketFromRaw: %v (raw=%q)", err, raw)
	}
	if err := schema.Validate(ticket); err != nil {
		t.Errorf("Validate: %v", err)
	}
	if ticket.Status != schema.StatusDone {
		t.Errorf("Status = %q, want done", ticket.Status)
	}
	if ticket.ActualContextWindow != 42000 {
		t.Errorf("ActualContextWindow = %d, want 42000", ticket.ActualContextWindow)
	}
	if ticket.ElapsedTime != 10 {
		t.Errorf("ElapsedTime = %d, want 10 (untouched)", ticket.ElapsedTime)
	}
	if !strings.Contains(raw, "Body.") {
		t.Errorf("body content lost:\n%s", raw)
	}
}

// TestClaimThenMarkDone_FrontmatterTicket_RoundTripsThroughSchema exercises
// the same claim -> done sequence as
// TestClaimThenMarkDone_RoundTripsThroughParseTicket, but for a
// frontmatter-format ticket.
func TestClaimThenMarkDone_FrontmatterTicket_RoundTripsThroughSchema(t *testing.T) {
	path := writeTicket(t, "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# Ticket\n\nBody.\n")

	if err := Claim(path); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	ticket, err := schema.ParseTicketFromRaw(mustRead(t, path), path)
	if err != nil {
		t.Fatalf("ParseTicketFromRaw: %v", err)
	}
	if ticket.Status != schema.StatusClaimed {
		t.Errorf("after Claim, Status = %q, want claimed", ticket.Status)
	}

	if err := MarkDone(path); err != nil {
		t.Fatalf("MarkDone: %v", err)
	}
	ticket, err = schema.ParseTicketFromRaw(mustRead(t, path), path)
	if err != nil {
		t.Fatalf("ParseTicketFromRaw: %v", err)
	}
	if ticket.Status != schema.StatusDone {
		t.Errorf("after MarkDone, Status = %q, want done", ticket.Status)
	}
}

// TestSetStatus_ConcurrentWritesAndReads_NeverExposesATornFile guards
// against a real race the scheduler hits under --max-parallel: one
// goroutine rewriting a ticket's Status: line (Claim/MarkDone) while
// another (the frontier scan) reads the same file. An in-place os.WriteFile
// truncates before it writes, so a concurrent reader can observe an empty
// or partial file; SetStatus must instead write via a temp file plus
// rename so every read sees a complete, valid ticket.
func TestSetStatus_ConcurrentWritesAndReads_NeverExposesATornFile(t *testing.T) {
	path := writeTicket(t, "# Ticket\n\n**Status:** open\n\nSome body text.\n")

	var wg sync.WaitGroup
	stop := make(chan struct{})
	readErrs := make(chan string, 1000)

	wg.Go(func() {
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			value := "claimed"
			if i%2 == 1 {
				value = "done"
			}
			if err := SetStatus(path, value); err != nil {
				t.Errorf("SetStatus: %v", err)
				return
			}
		}
	})

	for range 8 {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				raw, err := os.ReadFile(path)
				if err != nil {
					continue // a rename mid-open is a valid miss, not a torn read
				}
				if len(raw) == 0 || !strings.Contains(string(raw), "Some body text.") {
					select {
					case readErrs <- string(raw):
					default:
					}
				}
			}
		})
	}

	time.Sleep(200 * time.Millisecond)
	close(stop)
	wg.Wait()

	select {
	case got := <-readErrs:
		t.Errorf("read a torn/incomplete file mid-write: %q", got)
	default:
	}
}
