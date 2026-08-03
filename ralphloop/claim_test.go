package ralphloop

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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

// writeFrontmatterTicket writes a minimal valid ticket file with the given
// status and returns its path.
func writeFrontmatterTicket(t *testing.T, status string) string {
	t.Helper()
	return writeTicket(t, "---\nid: \"01\"\nstatus: "+status+"\ntype: task\n---\n# Ticket\n\nBody text.\n")
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return string(raw)
}

func mustParse(t *testing.T, path string) schema.Ticket {
	t.Helper()
	ticket, err := schema.ParseTicket(path)
	if err != nil {
		t.Fatalf("schema.ParseTicket: %v", err)
	}
	return ticket
}

func TestClaim_RewritesStatus(t *testing.T) {
	path := writeFrontmatterTicket(t, "open")
	if err := Claim(path); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if got := mustParse(t, path).Status; got != schema.StatusClaimed {
		t.Errorf("Status = %q, want %q", got, schema.StatusClaimed)
	}
}

func TestClaim_PreservesOtherFrontmatterFieldsAndBody(t *testing.T) {
	original := "---\nid: \"01\"\nstatus: open\nblocked_by: [\"02\", \"03\"]\ntype: task\n---\n" +
		"# Ticket\n\n- [ ] some criterion\n- [ ] another **bold** criterion\n\nTrailing prose with `code`.\n"
	path := writeTicket(t, original)
	if err := Claim(path); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	got := mustParse(t, path)
	if got.Status != schema.StatusClaimed {
		t.Errorf("Status = %q, want %q", got.Status, schema.StatusClaimed)
	}
	if want := []schema.TicketID{"02", "03"}; len(got.BlockedBy) != 2 || got.BlockedBy[0] != want[0] || got.BlockedBy[1] != want[1] {
		t.Errorf("BlockedBy = %v, want %v", got.BlockedBy, want)
	}
	if body := schema.ParseBody(mustRead(t, path)); !strings.Contains(body, "some criterion") || !strings.Contains(body, "Trailing prose") {
		t.Errorf("body = %q, want original body content preserved", body)
	}
}

func TestMarkDone_RewritesStatus(t *testing.T) {
	path := writeFrontmatterTicket(t, "claimed")
	if err := MarkDone(path); err != nil {
		t.Fatalf("MarkDone: %v", err)
	}
	if got := mustParse(t, path).Status; got != schema.StatusDone {
		t.Errorf("Status = %q, want %q", got, schema.StatusDone)
	}
}

func TestMarkDoneWithMetadata_SetsStatusAndActualContextWindow(t *testing.T) {
	path := writeFrontmatterTicket(t, "claimed")
	if err := MarkDoneWithMetadata(path, 42000, 2, "sess-123"); err != nil {
		t.Fatalf("MarkDoneWithMetadata: %v", err)
	}
	got := mustParse(t, path)
	if got.Status != schema.StatusDone {
		t.Errorf("Status = %q, want %q", got.Status, schema.StatusDone)
	}
	if got.ActualContextWindow != 42000 {
		t.Errorf("ActualContextWindow = %d, want 42000", got.ActualContextWindow)
	}
	if got.Compactions != 2 {
		t.Errorf("Compactions = %d, want 2", got.Compactions)
	}
}

func TestClaimThenMarkDone_UpdatesStatus(t *testing.T) {
	path := writeFrontmatterTicket(t, "open")

	if err := Claim(path); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if got := mustParse(t, path).Status; got != schema.StatusClaimed {
		t.Errorf("after Claim, Status = %q, want %q", got, schema.StatusClaimed)
	}

	if err := MarkDone(path); err != nil {
		t.Fatalf("MarkDone: %v", err)
	}
	if got := mustParse(t, path).Status; got != schema.StatusDone {
		t.Errorf("after MarkDone, Status = %q, want %q", got, schema.StatusDone)
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
	if err := MarkDoneWithMetadata(path, 42000, 0, "sess-123"); err != nil {
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

// TestClaim_NoFrontmatterReturnsError verifies ticket 04: with the old
// bold-line-regex format retired, a ticket file with no YAML frontmatter is
// no longer a valid ticket at all, so Claim must return an error rather than
// silently line-splicing a Status: line into it.
func TestClaim_NoFrontmatterReturnsError(t *testing.T) {
	path := writeTicket(t, "# Ticket\n\nJust a body, no frontmatter.\n")
	if err := Claim(path); err == nil {
		t.Error("Claim(file with no frontmatter) = nil error, want error")
	}
}

// TestSetStatus_ConcurrentWritesAndReads_NeverExposesATornFile guards
// against a real race the scheduler hits under --max-parallel: one
// goroutine rewriting a ticket's status field (Claim/MarkDone) while
// another (the frontier scan) reads the same file. An in-place os.WriteFile
// truncates before it writes, so a concurrent reader can observe an empty
// or partial file; SetStatus must instead write via a temp file plus
// rename so every read sees a complete, valid ticket.
func TestSetStatus_ConcurrentWritesAndReads_NeverExposesATornFile(t *testing.T) {
	path := writeFrontmatterTicket(t, "open")

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
				if len(raw) == 0 || !strings.Contains(string(raw), "Body text.") {
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
