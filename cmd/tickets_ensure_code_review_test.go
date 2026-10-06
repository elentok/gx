package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets/schema"
)

func TestResolveEpicArg_BareNameAndFullPathResolveToSameEpic(t *testing.T) {
	store := isolateTicketStore(t)
	repoDir := testutil.TempRepo(t)
	project := addProject(t, store, "mine", repoDir)
	epicDir := filepath.Join(project, "widget-epic")
	testutil.Mkdir(t, epicDir)

	bareResolved := resolveEpicArg("widget-epic", repoDir)
	fullResolved := resolveEpicArg(epicDir, repoDir)

	if bareResolved != epicDir {
		t.Errorf("resolveEpicArg(bare name) = %q, want %q", bareResolved, epicDir)
	}
	if fullResolved != epicDir {
		t.Errorf("resolveEpicArg(full path) = %q, want it returned unchanged", fullResolved)
	}
}

func TestResolveEpicArg_UnknownBareNameReturnedUnchanged(t *testing.T) {
	store := isolateTicketStore(t)
	repoDir := testutil.TempRepo(t)
	addProject(t, store, "mine", repoDir)

	resolved := resolveEpicArg("no-such-epic", repoDir)

	if resolved != "no-such-epic" {
		t.Errorf("resolveEpicArg(unknown name) = %q, want it returned unchanged", resolved)
	}
}

func TestCompleteEpicNames_ListsEpicsExcludingDotDirectories(t *testing.T) {
	store := isolateTicketStore(t)
	repoDir := testutil.TempRepo(t)
	project := addProject(t, store, "mine", repoDir)
	testutil.Mkdir(t, filepath.Join(project, "bugs-05"))
	testutil.Mkdir(t, filepath.Join(project, "widget-epic"))
	testutil.Mkdir(t, filepath.Join(project, ".archive"))

	names, err := completeEpicNames(repoDir)
	if err != nil {
		t.Fatalf("completeEpicNames: %v", err)
	}

	want := map[string]bool{"bugs-05": true, "widget-epic": true}
	if len(names) != len(want) {
		t.Fatalf("names = %v, want exactly %v", names, want)
	}
	for _, name := range names {
		if !want[name] {
			t.Errorf("unexpected epic name %q in completion list", name)
		}
	}
}

func TestRunTicketsEnsureCodeReview_NoopWhenCodeReviewTicketExists(t *testing.T) {
	t.Parallel()
	scratchDir := t.TempDir()
	epicPath := filepath.Join(scratchDir, "widget-epic")
	issuesDir := filepath.Join(epicPath, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("mkdir issues: %v", err)
	}
	writeTicket(t, filepath.Join(issuesDir, "01-do-thing.md"), "01", "done", "implement")
	writeTicket(t, filepath.Join(issuesDir, "02-review.md"), "02", "open", "code-review")

	var stdout bytes.Buffer
	if err := runTicketsEnsureCodeReview(epicPath, &stdout); err != nil {
		t.Fatalf("runTicketsEnsureCodeReview: %v", err)
	}

	entries, err := os.ReadDir(issuesDir)
	if err != nil {
		t.Fatalf("read issues dir: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("issues dir has %d entries, want 2 (no stub written)", len(entries))
	}
	if !strings.Contains(stdout.String(), "already has a code-review ticket") {
		t.Errorf("stdout = %q, want it to mention the existing code-review ticket", stdout.String())
	}
}

func TestRunTicketsEnsureCodeReview_CreatesValidStubWhenNoneExists(t *testing.T) {
	t.Parallel()
	scratchDir := t.TempDir()
	epicPath := filepath.Join(scratchDir, "widget-epic")
	issuesDir := filepath.Join(epicPath, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("mkdir issues: %v", err)
	}
	writeTicket(t, filepath.Join(issuesDir, "01-do-thing.md"), "01", "done", "implement")
	writeTicket(t, filepath.Join(issuesDir, "03-do-other-thing.md"), "03", "done", "implement")

	var stdout bytes.Buffer
	if err := runTicketsEnsureCodeReview(epicPath, &stdout); err != nil {
		t.Fatalf("runTicketsEnsureCodeReview: %v", err)
	}

	stubPath := filepath.Join(issuesDir, "04-code-review.md")
	ticket, err := schema.ParseTicket(stubPath)
	if err != nil {
		t.Fatalf("stub ticket %s failed validate: %v", stubPath, err)
	}
	if ticket.Type != schema.TypeCodeReview {
		t.Errorf("stub ticket type = %q, want %q", ticket.Type, schema.TypeCodeReview)
	}
	if ticket.Status != schema.StatusOpen {
		t.Errorf("stub ticket status = %q, want %q", ticket.Status, schema.StatusOpen)
	}
	if ticket.ID != "04" {
		t.Errorf("stub ticket id = %q, want next sequential id 04", ticket.ID)
	}
}

func TestExecute_TicketsEnsureCodeReview_StubLandsInStore(t *testing.T) {
	store := isolateTicketStore(t)
	repoDir := testutil.TempRepo(t)
	project := addProject(t, store, "mine", repoDir)
	issuesDir := filepath.Join(project, "widget-epic", "issues")
	testutil.Mkdir(t, issuesDir)
	writeTicket(t, filepath.Join(issuesDir, "01-do-thing.md"), "01", "done", "implement")

	if _, err := runIn(t, repoDir, "tickets", "ensure-code-review", "widget-epic"); err != nil {
		t.Fatalf("ensure-code-review: %v", err)
	}

	if _, err := schema.ParseTicket(filepath.Join(issuesDir, "02-code-review.md")); err != nil {
		t.Fatalf("stub not written inside the store: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoDir, ".scratch")); !os.IsNotExist(err) {
		t.Errorf("legacy .scratch dir was touched: %v", err)
	}
}

// writeTicket writes a minimal valid ticket fixture with the given id,
// status, and type.
func writeTicket(t *testing.T, path, id, status, ticketType string) {
	t.Helper()
	content := "---\nid: \"" + id + "\"\nstatus: " + status + "\ntype: " + ticketType + "\n---\n\n# " + id + "\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write ticket %s: %v", path, err)
	}
}
