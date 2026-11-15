package recovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func verifyNote(t *testing.T, body, head string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "01-verify.md")
	if err := os.WriteFile(path, []byte("---\ntype: verify\n---\n# T\n\n"+body), 0o644); err != nil {
		t.Fatal(err)
	}
	return VerifyResumeNote(tickets.Ticket{Type: string(schema.TypeVerify), Path: path}, head)
}

func TestVerifyResumeNote_StoriesSectionOwnsTheList(t *testing.T) {
	t.Parallel()
	stories := schema.SetSection("", schema.StoriesHeading, schema.StoriesSection([]string{"Story 1", "Story 2", "Mock states: empty"}))
	// Story 1 stands at HEAD, story 2 was recorded elsewhere, the Mock unit has no
	// entry. Hand-trimming ## Verified (here: no entry for story 2 either) must not
	// drop a unit from the re-check group.
	body := schema.AppendVerified(stories, schema.VerifiedEntry{Story: "Story 1", Verdict: "pass", Commit: "head1"})
	got := verifyNote(t, body, "head1")

	skipAt, recheckAt := strings.Index(got, "Skip these stories"), strings.Index(got, "Re-check these stories")
	if skipAt < 0 || recheckAt < skipAt {
		t.Fatalf("want skip then re-check groups:\n%s", got)
	}
	skip, recheck := got[skipAt:recheckAt], got[recheckAt:]
	if !strings.Contains(skip, "- Story 1 ") || strings.Contains(recheck, "Story 1") {
		t.Errorf("Story 1 belongs only under skip:\n%s", got)
	}
	for _, u := range []string{"- Story 2", "- Mock states: empty"} {
		if !strings.Contains(recheck, u) {
			t.Errorf("recheck lacks %q:\n%s", u, got)
		}
	}
	if !strings.Contains(got, VerifyEveryStory) {
		t.Errorf("note lacks the scope sentence:\n%s", got)
	}
}

func TestVerifyResumeNote_StoriesWithoutEntriesListsEveryUnit(t *testing.T) {
	t.Parallel()
	body := schema.SetSection("", schema.StoriesHeading, schema.StoriesSection([]string{"Story 1", "Story 2"}))
	got := verifyNote(t, body, "head1")
	if strings.Contains(got, "Skip these stories") || !strings.Contains(got, "- Story 1") || !strings.Contains(got, "- Story 2") {
		t.Errorf("want both units under re-check:\n%s", got)
	}
}

func TestVerifyResumeNote_NoStoriesSectionKeepsVerifiedSource(t *testing.T) {
	t.Parallel()
	body := schema.AppendVerified("", schema.VerifiedEntry{Story: "43", Verdict: "pass", Commit: "head1"})
	got := verifyNote(t, body, "head1")
	if !strings.Contains(got, VerifyEveryStory) || !strings.Contains(got, "Skip these stories") || !strings.Contains(got, "- 43 ") {
		t.Errorf("note changed for a ticket without ## Stories:\n%s", got)
	}
	if verifyNote(t, "", "head1") != "" {
		t.Error("a ticket with neither section gets no note")
	}
}
