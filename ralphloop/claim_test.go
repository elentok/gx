package ralphloop

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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
