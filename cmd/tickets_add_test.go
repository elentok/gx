package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/elentok/gx/repair"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func TestRunTicketsAdd_FlatSibling(t *testing.T) {
	t.Parallel()
	scratchDir := t.TempDir()
	epicPath := filepath.Join(scratchDir, "widget-epic")
	issuesDir := filepath.Join(epicPath, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("mkdir issues: %v", err)
	}
	writeTicket(t, filepath.Join(issuesDir, "01-do-thing.md"), "01", "done", "implement")
	writeTicket(t, filepath.Join(issuesDir, "02-do-other-thing.md"), "02", "done", "implement")
	writeTicket(t, filepath.Join(issuesDir, "03-do-third-thing.md"), "03", "done", "implement")

	var stdout bytes.Buffer
	if err := runTicketsAdd(deps{env: envFor(t)}, epicPath, "", "do-fourth-thing", &stdout); err != nil {
		t.Fatalf("runTicketsAdd: %v", err)
	}

	wantPath := filepath.Join(issuesDir, "04-do-fourth-thing.md")
	if got, want := strings.TrimSpace(stdout.String()), repair.EpicTicketLabel(epicPath, "04"); got != want {
		t.Fatalf("stdout = %q, want canonical address %q", got, want)
	}
	if got := addedPath(t, epicPath, stdout.String()); got != wantPath {
		t.Fatalf("address %q resolves to %q, want %q", stdout.String(), got, wantPath)
	}
	ticket, err := schema.ParseTicket(wantPath)
	if err != nil {
		t.Fatalf("stub ticket failed validation: %v", err)
	}
	if ticket.ID != "04" {
		t.Errorf("stub ticket id = %q, want %q", ticket.ID, "04")
	}
	if ticket.Type != schema.TypeImplement {
		t.Errorf("stub ticket type = %q, want %q", ticket.Type, schema.TypeImplement)
	}
}

func TestRunTicketsAddBody_NewEpicWritesTicketMD(t *testing.T) {
	t.Parallel()
	epicPath := filepath.Join(t.TempDir(), "new-epic")

	var stdout bytes.Buffer
	if err := runTicketsAddBody(deps{env: envFor(t)}, epicPath, "", "x", "# X\n", addFrontmatter{}, false, &stdout, nil); err != nil {
		t.Fatalf("runTicketsAddBody: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(epicPath, "ticket.md"))
	if err != nil {
		t.Fatalf("epic ticket.md: %v", err)
	}
	if want := "---\nstatus: open\n---\n\n# new-epic\n"; string(raw) != want {
		t.Errorf("epic ticket.md = %q, want %q", raw, want)
	}
	if _, err := schema.ParseTicket(addedPath(t, epicPath, stdout.String())); err != nil {
		t.Errorf("added ticket: %v", err)
	}
}

func TestRunTicketsAddBody_ExistingEpicKeepsTicketMD(t *testing.T) {
	t.Parallel()
	epicPath := filepath.Join(t.TempDir(), "widget-epic")
	testutil.Mkdir(t, filepath.Join(epicPath, "issues"))
	want := "---\nstatus: draft\nbase: main\n---\n\n# Widgets\n\nKeep me.\n"
	testutil.WriteFile(t, epicPath, "ticket.md", want)

	var stdout bytes.Buffer
	if err := runTicketsAddBody(deps{env: envFor(t)}, epicPath, "", "x", "# X\n", addFrontmatter{}, false, &stdout, nil); err != nil {
		t.Fatalf("runTicketsAddBody: %v", err)
	}

	if raw, _ := os.ReadFile(filepath.Join(epicPath, "ticket.md")); string(raw) != want {
		t.Errorf("epic ticket.md = %q, want unchanged %q", raw, want)
	}
}

func TestRunTicketsAdd_ExistingEpicWithoutTicketMDIsRepaired(t *testing.T) {
	t.Parallel()
	projectDir := t.TempDir()
	epicPath := filepath.Join(projectDir, "bare-epic")
	testutil.Mkdir(t, filepath.Join(epicPath, "issues"))

	var stdout bytes.Buffer
	if err := runTicketsAdd(deps{env: envFor(t)}, epicPath, "", "x", &stdout); err != nil {
		t.Fatalf("runTicketsAdd: %v", err)
	}

	if err := tickets.ValidateProject(projectDir); err != nil {
		t.Errorf("ValidateProject after add: %v", err)
	}
}

func TestRunTicketsAdd_WritesStatusDraft(t *testing.T) {
	t.Parallel()
	scratchDir := t.TempDir()
	epicPath := filepath.Join(scratchDir, "widget-epic")
	issuesDir := filepath.Join(epicPath, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("mkdir issues: %v", err)
	}
	testutil.EnsureEpicTicketMD(t, epicPath)

	var stdout bytes.Buffer
	if err := runTicketsAdd(deps{env: envFor(t)}, epicPath, "", "do-thing", &stdout); err != nil {
		t.Fatalf("runTicketsAdd: %v", err)
	}

	gotPath := addedPath(t, epicPath, stdout.String())
	ticket, err := schema.ParseTicket(gotPath)
	if err != nil {
		t.Fatalf("stub ticket failed validation: %v", err)
	}
	if ticket.Status != schema.StatusDraft {
		t.Errorf("stub ticket status = %q, want %q", ticket.Status, schema.StatusDraft)
	}
	if want := strings.TrimSpace(stdout.String()); ticket.OriginalAddress != want || ticket.CreatedAt.IsZero() {
		t.Errorf("stub created_at/original_address = %v/%q, want set/%q", ticket.CreatedAt, ticket.OriginalAddress, want)
	}
}

func TestRunTicketsAddBody_StampsCreatedAndOriginalAddress(t *testing.T) {
	t.Parallel()
	epicPath := filepath.Join(t.TempDir(), "widget-epic")
	testutil.Mkdir(t, filepath.Join(epicPath, "issues"))
	testutil.EnsureEpicTicketMD(t, epicPath)

	for _, parent := range []string{"", "01"} {
		var stdout bytes.Buffer
		if err := runTicketsAddBody(deps{env: envFor(t)}, epicPath, parent, "x", "# X\n", addFrontmatter{}, false, &stdout, nil); err != nil {
			t.Fatalf("runTicketsAddBody(parent %q): %v", parent, err)
		}
		printed := strings.TrimSpace(stdout.String())
		tk, err := schema.ParseTicket(addedPath(t, epicPath, printed))
		if err != nil {
			t.Fatal(err)
		}
		if tk.OriginalAddress != printed || tk.CreatedAt.IsZero() {
			t.Errorf("parent %q: created_at/original_address = %v/%q, want set/%q", parent, tk.CreatedAt, tk.OriginalAddress, printed)
		}
	}
}

func TestRunTicketsAdd_EmptySlugFails(t *testing.T) {
	t.Parallel()
	scratchDir := t.TempDir()
	epicPath := filepath.Join(scratchDir, "widget-epic")
	issuesDir := filepath.Join(epicPath, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("mkdir issues: %v", err)
	}

	var stdout bytes.Buffer
	err := runTicketsAdd(deps{env: envFor(t)}, epicPath, "", "", &stdout)
	if err == nil {
		t.Fatal("runTicketsAdd with empty slug: want error, got nil")
	}
	entries, readErr := os.ReadDir(issuesDir)
	if readErr != nil {
		t.Fatalf("read issues dir: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("issues dir = %v, want empty (no stub written on validation failure)", entries)
	}
}

func TestRunTicketsAdd_LetteredChild(t *testing.T) {
	t.Parallel()
	scratchDir := t.TempDir()
	epicPath := filepath.Join(scratchDir, "widget-epic")
	issuesDir := filepath.Join(epicPath, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("mkdir issues: %v", err)
	}
	writeTicket(t, filepath.Join(issuesDir, "12-parent.md"), "12", "done", "implement")

	var stdout bytes.Buffer
	if err := runTicketsAdd(deps{env: envFor(t)}, epicPath, "12", "child-a", &stdout); err != nil {
		t.Fatalf("runTicketsAdd: %v", err)
	}
	firstPath := addedPath(t, epicPath, stdout.String())
	if firstPath != filepath.Join(issuesDir, "12a-child-a.md") {
		t.Fatalf("stdout = %q, want %q", firstPath, filepath.Join(issuesDir, "12a-child-a.md"))
	}

	stdout.Reset()
	if err := runTicketsAdd(deps{env: envFor(t)}, epicPath, "12", "child-b", &stdout); err != nil {
		t.Fatalf("runTicketsAdd (second call): %v", err)
	}
	secondPath := addedPath(t, epicPath, stdout.String())
	if secondPath != filepath.Join(issuesDir, "12b-child-b.md") {
		t.Fatalf("stdout = %q, want %q", secondPath, filepath.Join(issuesDir, "12b-child-b.md"))
	}
}

func TestRunTicketsAdd_WritesParentFrontmatter(t *testing.T) {
	t.Parallel()
	scratchDir := t.TempDir()
	epicPath := filepath.Join(scratchDir, "widget-epic")
	issuesDir := filepath.Join(epicPath, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("mkdir issues: %v", err)
	}
	writeTicket(t, filepath.Join(issuesDir, "12-parent.md"), "12", "done", "implement")

	var stdout bytes.Buffer
	if err := runTicketsAdd(deps{env: envFor(t)}, epicPath, "12", "child-a", &stdout); err != nil {
		t.Fatalf("runTicketsAdd: %v", err)
	}
	gotPath := addedPath(t, epicPath, stdout.String())

	ticket, err := schema.ParseTicket(gotPath)
	if err != nil {
		t.Fatalf("stub ticket failed validation: %v", err)
	}
	if ticket.Parent == nil || *ticket.Parent != "12" {
		t.Errorf("stub ticket parent = %v, want \"12\"", ticket.Parent)
	}
}

// TestRunTicketsAdd_LeavesParentUntouched covers the contraction's other
// half: with children derived from `parent`, adding a fork writes exactly one
// file and leaves the parent byte-identical, so there is no backfill left to
// go missing.
func TestRunTicketsAdd_LeavesParentUntouched(t *testing.T) {
	t.Parallel()
	scratchDir := t.TempDir()
	epicPath := filepath.Join(scratchDir, "widget-epic")
	issuesDir := filepath.Join(epicPath, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("mkdir issues: %v", err)
	}
	parentPath := filepath.Join(issuesDir, "12-parent.md")
	writeTicket(t, parentPath, "12", "done", "implement")

	before, err := os.ReadFile(parentPath)
	if err != nil {
		t.Fatalf("reading parent ticket: %v", err)
	}

	var stdout bytes.Buffer
	if err := runTicketsAdd(deps{env: envFor(t)}, epicPath, "12", "child-a", &stdout); err != nil {
		t.Fatalf("runTicketsAdd: %v", err)
	}

	after, err := os.ReadFile(parentPath)
	if err != nil {
		t.Fatalf("re-reading parent ticket: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("parent ticket rewritten:\nbefore %q\nafter  %q", before, after)
	}
}

func TestRunTicketsAdd_NumericLevelPastLetteredParent(t *testing.T) {
	t.Parallel()
	scratchDir := t.TempDir()
	epicPath := filepath.Join(scratchDir, "widget-epic")
	issuesDir := filepath.Join(epicPath, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("mkdir issues: %v", err)
	}
	writeTicket(t, filepath.Join(issuesDir, "12b-child.md"), "12b", "done", "implement")

	var stdout bytes.Buffer
	if err := runTicketsAdd(deps{env: envFor(t)}, epicPath, "12b", "grandchild-1", &stdout); err != nil {
		t.Fatalf("runTicketsAdd: %v", err)
	}
	firstPath := addedPath(t, epicPath, stdout.String())
	if firstPath != filepath.Join(issuesDir, "12b1-grandchild-1.md") {
		t.Fatalf("stdout = %q, want %q", firstPath, filepath.Join(issuesDir, "12b1-grandchild-1.md"))
	}

	stdout.Reset()
	if err := runTicketsAdd(deps{env: envFor(t)}, epicPath, "12b", "grandchild-2", &stdout); err != nil {
		t.Fatalf("runTicketsAdd (second call): %v", err)
	}
	secondPath := addedPath(t, epicPath, stdout.String())
	if secondPath != filepath.Join(issuesDir, "12b2-grandchild-2.md") {
		t.Fatalf("stdout = %q, want %q", secondPath, filepath.Join(issuesDir, "12b2-grandchild-2.md"))
	}
}

func TestRunTicketsAdd_LetteredNumericParentAllocatesNextSibling(t *testing.T) {
	t.Parallel()
	scratchDir := t.TempDir()
	epicPath := filepath.Join(scratchDir, "widget-epic")
	issuesDir := filepath.Join(epicPath, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("mkdir issues: %v", err)
	}
	writeTicket(t, filepath.Join(issuesDir, "12b1-child.md"), "12b1", "done", "implement")

	var stdout bytes.Buffer
	if err := runTicketsAdd(deps{env: envFor(t)}, epicPath, "12b1", "sibling", &stdout); err != nil {
		t.Fatalf("runTicketsAdd(parent=12b1): %v", err)
	}
	gotPath := addedPath(t, epicPath, stdout.String())
	if gotPath != filepath.Join(issuesDir, "12b2-sibling.md") {
		t.Fatalf("stdout = %q, want %q", gotPath, filepath.Join(issuesDir, "12b2-sibling.md"))
	}
}

func TestRunTicketsAdd_ConcurrentCallsAllocateDistinctIDs(t *testing.T) {
	t.Parallel()
	scratchDir := t.TempDir()
	epicPath := filepath.Join(scratchDir, "widget-epic")
	issuesDir := filepath.Join(epicPath, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("mkdir issues: %v", err)
	}
	testutil.EnsureEpicTicketMD(t, epicPath)

	const n = 15
	paths := make([]string, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var buf bytes.Buffer
			errs[i] = runTicketsAdd(deps{env: envFor(t)}, epicPath, "", "concurrent", &buf)
			paths[i] = strings.TrimSpace(buf.String())
		}(i)
	}
	wg.Wait()

	seen := map[string]bool{}
	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
		if seen[paths[i]] {
			t.Fatalf("duplicate allocated path %q", paths[i])
		}
		seen[paths[i]] = true
	}
	if len(seen) != n {
		t.Fatalf("expected %d unique paths, got %d", n, len(seen))
	}
}

// addedPath resolves the address `add` printed back to the stub's file.
func addedPath(t *testing.T, epicPath, out string) string {
	t.Helper()
	addr := strings.TrimSpace(out)
	id := addr[strings.LastIndex(addr, "/")+1:]
	matches, err := filepath.Glob(filepath.Join(epicPath, "issues", id+"-*.md"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("address %q resolves to files %v (err %v), want exactly one", addr, matches, err)
	}
	return matches[0]
}

func TestExecute_TicketsAdd_CrossProject(t *testing.T) {
	repo, project := setupShowProject(t)
	theirs := addProject(t, filepath.Dir(project), "theirs", testutil.TempRepo(t))
	cwd := t.TempDir()

	out, err := runIn(t, repo, "tickets", "add", "theirs:inbox", "--slug", "hello", "--body",
		"---\nstatus: draft\ncategory: idea\n---\n\n# Hello\n\ntext\n")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if got := strings.TrimSpace(out); got != "theirs:inbox/01" {
		t.Errorf("printed %q, want theirs:inbox/01", got)
	}
	tk, err := schema.ParseTicket(filepath.Join(theirs, "inbox", "issues", "01-hello.md"))
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != schema.StatusDraft || tk.Category != schema.CategoryIdea {
		t.Errorf("status/category = %s/%s, want draft/idea", tk.Status, tk.Category)
	}

	_, err = runIn(t, cwd, "tickets", "add", "nope:inbox", "--slug", "x")
	var ae *tickets.AddressError
	if !errors.As(err, &ae) || ae.Code != tickets.CodeUnknownProject {
		t.Errorf("unknown project err = %v, want %s", err, tickets.CodeUnknownProject)
	}
	if _, statErr := os.Stat(filepath.Join(cwd, "nope:inbox")); !os.IsNotExist(statErr) {
		t.Errorf("stray directory created in cwd: %v", statErr)
	}
}

func TestExecute_TicketsAdd_FieldFlags(t *testing.T) {
	repo, project := setupShowProject(t)

	if _, err := runIn(t, repo, "tickets", "add", "mine:inbox", "--slug", "first", "--body", "# First\n"); err != nil {
		t.Fatalf("add first: %v", err)
	}
	if _, err := runIn(t, repo, "tickets", "add", "mine:inbox", "--slug", "second", "--body", "# Second\n",
		"--blocked-by", "01", "--type", "research", "--expected-context-window", "60000",
		"--commitless", "true", "--category", "bug"); err != nil {
		t.Fatalf("add with flags: %v", err)
	}
	tk, err := schema.ParseTicket(filepath.Join(project, "inbox", "issues", "02-second.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(tk.BlockedBy) != 1 || tk.BlockedBy[0] != "01" || tk.Type != schema.TicketType("research") ||
		tk.ExpectedContextWindow != 60000 || !tk.Commitless || tk.Category != schema.CategoryBug ||
		tk.Status != schema.StatusOpen {
		t.Errorf("ticket = %+v, want the flags' fields, open", tk)
	}
}

func TestExecute_TicketsAdd_FieldFlagErrors(t *testing.T) {
	repo, project := setupShowProject(t)

	for name, args := range map[string][]string{
		"conflicts with body": {"--body", "---\ntype: research\n---\n\n# X\n", "--type", "research"},
		"bool conflict":       {"--body", "---\ncommitless: false\n---\n\n# X\n", "--commitless", "true"},
		"bad type":            {"--body", "# X\n", "--type", "nope"},
		"bad window":          {"--body", "# X\n", "--expected-context-window", "lots"},
		"window in K":         {"--body", "# X\n", "--expected-context-window", "57"},
		"bad commitless":      {"--body", "# X\n", "--commitless", "maybe"},
		"bad category":        {"--body", "# X\n", "--category", "nope"},
		"dangling blocked_by": {"--body", "# X\n", "--blocked-by", "07"},
		"bad flag on a stub":  {"--type", "nope"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := runIn(t, repo, append([]string{"tickets", "add", "mine:inbox", "--slug", "x"}, args...)...); err == nil {
				t.Errorf("add %v succeeded, want an error", args)
			}
		})
	}
	if matches, _ := filepath.Glob(filepath.Join(project, "inbox", "issues", "*.md")); len(matches) != 0 {
		t.Errorf("refused adds left tickets behind: %v", matches)
	}
}

func TestExecute_TicketsAdd_FollowUpsNeedsCategory(t *testing.T) {
	repo, project := setupShowProject(t)

	_, err := runIn(t, repo, "tickets", "add", "mine:follow-ups", "--slug", "x", "--body", "# X\n\ntext\n")
	var ae *tickets.AddressError
	if !errors.As(err, &ae) || ae.Code != tickets.CodeCategoryRequired {
		t.Fatalf("err = %v, want %s", err, tickets.CodeCategoryRequired)
	}
	if _, statErr := os.Stat(filepath.Join(project, "follow-ups")); !os.IsNotExist(statErr) {
		t.Errorf("follow-ups was created: %v", statErr)
	}

	if _, err := runIn(t, repo, "tickets", "add", "mine:follow-ups", "--slug", "x", "--body",
		"---\ncategory: bug\n---\n\n# X\n\ntext\n"); err != nil {
		t.Errorf("add with category: %v", err)
	}
}
