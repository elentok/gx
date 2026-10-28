package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func TestResolveEpicArg_BareNameAndFullPathResolveToSameEpic(t *testing.T) {
	store := isolateTicketStore(t)
	repoDir := testutil.TempRepo(t)
	project := addProject(t, store, "mine", repoDir)
	epicDir := filepath.Join(project, "widget-epic")
	testutil.Mkdir(t, epicDir)

	bareResolved := resolveEpicArg(config.DirsFrom(envFor(t)), "widget-epic", repoDir)
	fullResolved := resolveEpicArg(config.DirsFrom(envFor(t)), epicDir, repoDir)

	if bareResolved != epicDir {
		t.Errorf("resolveEpicArg(bare name) = %q, want %q", bareResolved, epicDir)
	}
	if fullResolved != epicDir {
		t.Errorf("resolveEpicArg(full path) = %q, want it returned unchanged", fullResolved)
	}
}

func TestResolveEpicArg_ProjectEpicAddressResolvesInCurrentAndOtherProject(t *testing.T) {
	store := isolateTicketStore(t)
	repoDir := testutil.TempRepo(t)
	otherRepoDir := testutil.TempRepo(t)
	mine := addProject(t, store, "mine", repoDir)
	other := addProject(t, store, "other", otherRepoDir)
	testutil.Mkdir(t, filepath.Join(mine, "widget-epic"))
	testutil.Mkdir(t, filepath.Join(other, "gadget-epic"))

	if got, want := resolveEpicArg(config.DirsFrom(envFor(t)), "mine:widget-epic", repoDir), filepath.Join(mine, "widget-epic"); got != want {
		t.Errorf("resolveEpicArg(current project address) = %q, want %q", got, want)
	}
	if got, want := resolveEpicArg(config.DirsFrom(envFor(t)), "other:gadget-epic", repoDir), filepath.Join(other, "gadget-epic"); got != want {
		t.Errorf("resolveEpicArg(other project address) = %q, want %q", got, want)
	}
}

func TestResolveEpicArg_UnknownBareNameReturnedUnchanged(t *testing.T) {
	store := isolateTicketStore(t)
	repoDir := testutil.TempRepo(t)
	addProject(t, store, "mine", repoDir)

	resolved := resolveEpicArg(config.DirsFrom(envFor(t)), "no-such-epic", repoDir)

	if resolved != "no-such-epic" {
		t.Errorf("resolveEpicArg(unknown name) = %q, want it returned unchanged", resolved)
	}
}

func TestCompleteEpicNames_ListsEpicsExcludingDotDirectories(t *testing.T) {
	store := isolateTicketStore(t)
	repoDir := testutil.TempRepo(t)
	project := addProject(t, store, "mine", repoDir)
	testutil.EnsureEpicTicketMD(t, filepath.Join(project, "bugs-05"))
	testutil.EnsureEpicTicketMD(t, filepath.Join(project, "widget-epic"))
	testutil.Mkdir(t, filepath.Join(project, ".archive"))

	names, err := completeEpicNames(config.DirsFrom(envFor(t)), repoDir)
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
	if ticket.CreatedAt.IsZero() {
		t.Error("stub ticket has no created_at")
	}
	wantAddr := tickets.ProjectName(scratchDir) + ":widget-epic/04"
	if ticket.OriginalAddress != wantAddr {
		t.Errorf("stub original_address = %q, want %q", ticket.OriginalAddress, wantAddr)
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

func TestRunTicketsEnsureVerify_CreatesOneValidStubThenNoops(t *testing.T) {
	t.Parallel()
	epicPath := filepath.Join(t.TempDir(), "widget-epic")
	issuesDir := filepath.Join(epicPath, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("mkdir issues: %v", err)
	}
	writeTicket(t, filepath.Join(issuesDir, "01-do-thing.md"), "01", "done", "implement")

	var stdout bytes.Buffer
	if err := runTicketsEnsureVerify(epicPath, &stdout); err != nil {
		t.Fatalf("first run: %v", err)
	}
	ticket, err := schema.ParseTicket(filepath.Join(issuesDir, "02-verify.md"))
	if err != nil {
		t.Fatalf("stub failed validate: %v", err)
	}
	if ticket.Type != schema.TypeVerify || !ticket.IsCommitless() {
		t.Errorf("stub type = %q commitless = %v, want commitless verify", ticket.Type, ticket.IsCommitless())
	}

	stdout.Reset()
	if err := runTicketsEnsureVerify(epicPath, &stdout); err != nil {
		t.Fatalf("second run: %v", err)
	}
	entries, _ := os.ReadDir(issuesDir)
	if len(entries) != 2 {
		t.Errorf("issues dir has %d entries, want 2 (second run is a no-op)", len(entries))
	}
	if !strings.Contains(stdout.String(), "already has a verify ticket") {
		t.Errorf("stdout = %q, want it to mention the existing verify ticket", stdout.String())
	}
}

func TestRunTicketsEnsureVerify_StubGetsSeventyKBudgetAndCodeReviewKeepsThirtyK(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		run    func(string, io.Writer) error
		file   string
		budget int
	}{
		{runTicketsEnsureVerify, "02-verify.md", 70000},
		{runTicketsEnsureCodeReview, "02-code-review.md", 30000},
	} {
		epicPath := filepath.Join(t.TempDir(), "widget-epic")
		writeDoneImplTicket(t, epicPath)

		if err := tc.run(epicPath, io.Discard); err != nil {
			t.Fatalf("%s: %v", tc.file, err)
		}
		ticket, err := schema.ParseTicket(filepath.Join(epicPath, "issues", tc.file))
		if err != nil {
			t.Fatalf("parse %s: %v", tc.file, err)
		}
		if ticket.ExpectedContextWindow != tc.budget {
			t.Errorf("%s expected_context_window = %d, want %d", tc.file, ticket.ExpectedContextWindow, tc.budget)
		}
	}
}

func TestExecute_TicketsEnsureVerify_BodyNamesRepoSpecWithOrWithoutImplSuffix(t *testing.T) {
	store := isolateTicketStore(t)
	repoDir := testutil.TempRepo(t)
	project := addProject(t, store, "mine", repoDir)
	if err := os.MkdirAll(filepath.Join(repoDir, "docs", "specs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "docs", "specs", "widget.md"), []byte("# spec\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, epic := range []string{"widget-impl", "widget", "other-impl"} {
		writeDoneImplTicket(t, filepath.Join(project, epic))
	}

	for epic, wantSpec := range map[string]bool{"widget-impl": true, "widget": true, "other-impl": false} {
		if _, err := runIn(t, repoDir, "tickets", "ensure-verify", epic); err != nil {
			t.Fatalf("ensure-verify %s: %v", epic, err)
		}
		body, err := os.ReadFile(filepath.Join(project, epic, "issues", "02-verify.md"))
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(string(body), "docs/specs/widget.md"); got != wantSpec {
			t.Errorf("%s: body names repo spec = %v, want %v\n%s", epic, got, wantSpec, body)
		}
		if !wantSpec && strings.Contains(string(body), "Spec:") {
			t.Errorf("%s: body names a spec but none exists\n%s", epic, body)
		}
	}
}

func TestRunTicketsEnsureVerify_BodyNamesEpicOwnSpecMd(t *testing.T) {
	t.Parallel()
	epicPath := filepath.Join(t.TempDir(), "widget-epic")
	writeDoneImplTicket(t, epicPath)
	if err := os.WriteFile(filepath.Join(epicPath, "spec.md"), []byte("# spec\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := runTicketsEnsureVerify(epicPath, io.Discard); err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(filepath.Join(epicPath, "issues", "02-verify.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), filepath.Join(epicPath, "spec.md")) {
		t.Errorf("body does not name the epic's spec.md\n%s", body)
	}
}

// writeDoneImplTicket gives epicPath one done implement ticket, creating its
// issues dir.
func writeDoneImplTicket(t *testing.T, epicPath string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(epicPath, "issues"), 0755); err != nil {
		t.Fatal(err)
	}
	writeTicket(t, filepath.Join(epicPath, "issues", "01-do-thing.md"), "01", "done", "implement")
}

// writeTicket writes a minimal valid ticket fixture with the given id,
// status, and type.
func writeTicket(t *testing.T, path, id, status, ticketType string) {
	t.Helper()
	ensureIssueEpic(t, path)
	content := "---\nid: \"" + id + "\"\nstatus: " + status + "\ntype: " + ticketType + "\n---\n\n# " + id + "\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write ticket %s: %v", path, err)
	}
}
