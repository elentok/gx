package ralphloop

import (
	"os"
	"path/filepath"
	"regexp"
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
	t.Parallel()
	path := writeFrontmatterTicket(t, "open")
	if err := Claim(path); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if got := mustParse(t, path).Status; got != schema.StatusClaimed {
		t.Errorf("Status = %q, want %q", got, schema.StatusClaimed)
	}
}

func TestClaim_ClearsIterationStatus(t *testing.T) {
	t.Parallel()
	path := writeTicket(t, "---\nid: \"01\"\nstatus: claimed\niteration_status: finished\ntype: task\n---\n# Ticket\n\nBody.\n")
	if err := Claim(path); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	ticket := mustParse(t, path)
	if ticket.IterationStatus != "" {
		t.Errorf("IterationStatus = %q, want cleared", ticket.IterationStatus)
	}
	if ticket.Status != schema.StatusClaimed {
		t.Errorf("Status = %q, want %q", ticket.Status, schema.StatusClaimed)
	}
	if strings.Contains(mustRead(t, path), "iteration_status") {
		t.Errorf("ticket file = %q, want iteration_status omitted entirely", mustRead(t, path))
	}
}

func TestClaim_ClearsParkKind(t *testing.T) {
	t.Parallel()
	path := writeTicket(t, "---\nid: \"01\"\nstatus: needs-answer\npark_kind: zero-commit\ntype: task\n---\n# Ticket\n\nBody.\n")
	if err := Claim(path); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	ticket := mustParse(t, path)
	if ticket.ParkKind != "" {
		t.Errorf("ParkKind = %q, want cleared", ticket.ParkKind)
	}
	if strings.Contains(mustRead(t, path), "park_kind") {
		t.Errorf("ticket file = %q, want park_kind omitted entirely", mustRead(t, path))
	}
}

func TestClaim_PreservesOtherFrontmatterFieldsAndBody(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	path := writeFrontmatterTicket(t, "claimed")
	if err := MarkDone(path); err != nil {
		t.Fatalf("MarkDone: %v", err)
	}
	if got := mustParse(t, path).Status; got != schema.StatusDone {
		t.Errorf("Status = %q, want %q", got, schema.StatusDone)
	}
}

func TestMarkDoneWithMetadata_SetsStatusAndActualContextWindow(t *testing.T) {
	t.Parallel()
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

// TestClaim_DemotesNeedsRepairIntoDatedComments pins the ticket 19 behaviour:
// claiming a ticket carrying "## Needs Repair" moves that reason into a
// dated "## Comments" sub-entry and removes the "## Needs Repair" heading.
func TestClaim_DemotesNeedsRepairIntoDatedComments(t *testing.T) {
	t.Parallel()
	path := writeTicket(t, "---\nid: \"01\"\nstatus: needs-repair\ntype: task\n---\n"+
		"# Ticket\n\nBody text.\n\n## Needs Repair\n\nsomething broke\n")

	if err := Claim(path); err != nil {
		t.Fatalf("Claim: %v", err)
	}

	body := schema.ParseBody(mustRead(t, path))
	if strings.Contains(body, "\n## Needs Repair\n") {
		t.Errorf("body = %q, want ## Needs Repair heading removed", body)
	}
	if !strings.Contains(body, "## Comments") {
		t.Errorf("body = %q, want ## Comments heading present", body)
	}
	if !strings.Contains(body, "something broke") {
		t.Errorf("body = %q, want the retired reason preserved", body)
	}
	datePattern := regexp.MustCompile(`\*\*\d{4}-\d{2}-\d{2}\*\* — retired from`)
	if !datePattern.MatchString(body) {
		t.Errorf("body = %q, want a dated retirement line", body)
	}
}

// TestClaim_RepeatedClaimsDoNotStackDuplicateComments pins the fix for the
// live stacking bug: claiming a ticket that has already been demoted once
// (no "## Needs Repair" section left to demote) must not append a second
// entry.
func TestClaim_RepeatedClaimsDoNotStackDuplicateComments(t *testing.T) {
	t.Parallel()
	path := writeTicket(t, "---\nid: \"01\"\nstatus: needs-repair\ntype: task\n---\n"+
		"# Ticket\n\nBody text.\n\n## Needs Repair\n\nfirst failure\n")

	if err := Claim(path); err != nil {
		t.Fatalf("first Claim: %v", err)
	}
	afterFirst := schema.ParseBody(mustRead(t, path))

	if err := Claim(path); err != nil {
		t.Fatalf("second Claim: %v", err)
	}
	afterSecond := schema.ParseBody(mustRead(t, path))

	if afterFirst != afterSecond {
		t.Errorf("second Claim changed the body:\nfirst:  %q\nsecond: %q", afterFirst, afterSecond)
	}
	if n := strings.Count(afterSecond, "## Comments"); n != 1 {
		t.Errorf("## Comments appears %d times, want exactly 1", n)
	}
}

// TestClaim_AppendsSecondDemotionAlongsideFirst pins the "repeated" case
// this ticket actually cares about: a ticket parked, unparked, reclaimed,
// and parked again a second time must accumulate two dated entries under
// one "## Comments" heading, not two "## Comments" headings.
func TestClaim_AppendsSecondDemotionAlongsideFirst(t *testing.T) {
	t.Parallel()
	path := writeTicket(t, "---\nid: \"01\"\nstatus: needs-repair\ntype: task\n---\n"+
		"# Ticket\n\nBody text.\n\n## Needs Repair\n\nfirst failure\n")

	if err := Claim(path); err != nil {
		t.Fatalf("first Claim: %v", err)
	}
	if err := MarkNeedsRepairWithReason(path, "second failure", schema.NeedsRepairState{}); err != nil {
		t.Fatalf("MarkNeedsRepairWithReason: %v", err)
	}
	if err := Claim(path); err != nil {
		t.Fatalf("second Claim: %v", err)
	}

	body := schema.ParseBody(mustRead(t, path))
	if n := strings.Count(body, "## Comments"); n != 1 {
		t.Errorf("## Comments appears %d times, want exactly 1", n)
	}
	if !strings.Contains(body, "first failure") || !strings.Contains(body, "second failure") {
		t.Errorf("body = %q, want both retired reasons present", body)
	}
}

// TestClaim_UnparkedButUnclaimedKeepsNeedsRepairVisible pins that setting a
// parked ticket's status directly to open (the unpark gesture) does not by
// itself retire "## Needs Repair" — only Claim does. A person who unparks a
// ticket should still see what they were told, right up until it's claimed.
func TestClaim_UnparkedButUnclaimedKeepsNeedsRepairVisible(t *testing.T) {
	t.Parallel()
	path := writeTicket(t, "---\nid: \"01\"\nstatus: needs-repair\ntype: task\n---\n"+
		"# Ticket\n\nBody text.\n\n## Needs Repair\n\nsomething broke\n")

	if err := SetStatus(path, "open"); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}

	body := schema.ParseBody(mustRead(t, path))
	if !strings.Contains(body, "## Needs Repair") {
		t.Errorf("body = %q, want ## Needs Repair still present after unpark", body)
	}
}

// TestReattach_ClearIterationStatusDoesNotRetireNeedsRepair pins the ticket
// 19 behaviour change: the ordinary reattach path (a still-claimed ticket
// resumed via schema.ClearIterationStatus, see reattachIteration) does not
// go through Claim, so it must not retire "## Needs Repair" — only an
// actual claim does. A ticket can reattach several times within one claim;
// retiring here would fire repeatedly instead of exactly once at claim.
func TestReattach_ClearIterationStatusDoesNotRetireNeedsRepair(t *testing.T) {
	t.Parallel()
	path := writeTicket(t, "---\nid: \"01\"\nstatus: claimed\niteration_status: working\ntype: task\n---\n"+
		"# Ticket\n\nBody text.\n\n## Needs Repair\n\nsomething broke\n")

	if err := schema.ClearIterationStatus(path); err != nil {
		t.Fatalf("ClearIterationStatus: %v", err)
	}

	body := schema.ParseBody(mustRead(t, path))
	if !strings.Contains(body, "\n## Needs Repair\n") {
		t.Errorf("body = %q, want ## Needs Repair still present after reattach", body)
	}
}

func TestAppendSessionID_AppendsWithoutOverwriting(t *testing.T) {
	t.Parallel()
	path := writeTicket(t, "---\nid: \"01\"\nstatus: claimed\ntype: task\nsession_ids: [\"sess-1\"]\n---\n# Ticket\n\nBody.\n")

	if err := AppendSessionID(path, "sess-2"); err != nil {
		t.Fatalf("AppendSessionID: %v", err)
	}

	got := mustParse(t, path)
	want := []string{"sess-1", "sess-2"}
	if len(got.SessionIDs) != len(want) || got.SessionIDs[0] != want[0] || got.SessionIDs[1] != want[1] {
		t.Errorf("SessionIDs = %v, want %v", got.SessionIDs, want)
	}
}

func TestAppendSessionID_FirstEntryOnUnsetField(t *testing.T) {
	t.Parallel()
	path := writeFrontmatterTicket(t, "claimed")

	if err := AppendSessionID(path, "sess-1"); err != nil {
		t.Fatalf("AppendSessionID: %v", err)
	}

	got := mustParse(t, path)
	if want := []string{"sess-1"}; len(got.SessionIDs) != 1 || got.SessionIDs[0] != want[0] {
		t.Errorf("SessionIDs = %v, want %v", got.SessionIDs, want)
	}
}

func TestClaimThenMarkDone_UpdatesStatus(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// TestMarkNeedsAnswerWithReasonAndStub_WritesStatusReasonAndStub verifies
// ticket 14's pane-answered park write: status becomes needs-answer, and the
// body gains a "## Needs Answer" stub containing reason, which names the
// iteration label rather than a raw pane id — a label still resolves after a
// restart or reattach, and a pane id does not.
func TestMarkNeedsAnswerWithReasonAndStub_WritesStatusReasonAndStub(t *testing.T) {
	t.Parallel()
	path := writeFrontmatterTicket(t, "claimed")
	reason := "iter-01 is blocked on a prompt gx did not send; answer it in the pane"

	if err := MarkNeedsAnswerWithReasonAndStub(path, reason, schema.ParkKindBlockedPane); err != nil {
		t.Fatalf("MarkNeedsAnswerWithReasonAndStub: %v", err)
	}

	got := mustParse(t, path)
	if got.Status != schema.StatusNeedsAnswer {
		t.Errorf("Status = %q, want needs-answer", got.Status)
	}
	if got.ParkKind != schema.ParkKindBlockedPane {
		t.Errorf("ParkKind = %q, want blocked-pane", got.ParkKind)
	}
	body := schema.ParseBody(mustRead(t, path))
	if !strings.Contains(body, "## Needs Answer") {
		t.Errorf("body missing ## Needs Answer stub:\n%s", body)
	}
	if !strings.Contains(body, "iter-01") {
		t.Errorf("body does not name the iteration label iter-01:\n%s", body)
	}
	if !strings.Contains(body, "Body text.") {
		t.Errorf("body lost its original content:\n%s", body)
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
