package ticketsflow

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdtest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

const followUpSections = "## Source\n\nsession x\n\n## Problem\n\nThe thing breaks on empty input.\n\n## Suggested fix\n\nGuard it.\n"

func TestExecute_TicketsFollowUpAdd_FilesDraftResearch(t *testing.T) {
	repo, project := setupShowProject(t)
	theirs := addProject(t, filepath.Dir(project), "theirs", testutil.TempRepo(t))

	bug := "---\ncategory: bug\n---\n\n" + followUpSections + "\n## Evidence\n\nlog line\n"
	out, err := runIn(t, repo, "tickets", "follow-up", "add", "mine:follow-ups", "--body", bug)
	if err != nil {
		t.Fatalf("bug: %v", err)
	}
	if got := strings.TrimSpace(out); got != "mine:follow-ups/01" {
		t.Errorf("printed %q, want mine:follow-ups/01", got)
	}
	tk, err := schema.ParseTicket(cmdtest.AddedPath(t, filepath.Join(project, "follow-ups"), out))
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != schema.StatusDraft || tk.Type != schema.TypeResearch || tk.Category != schema.CategoryBug {
		t.Errorf("status/type/category = %s/%s/%s, want draft/research/bug", tk.Status, tk.Type, tk.Category)
	}
	if tk.OriginalAddress.String() != "mine:follow-ups/01" || tk.CreatedAt.IsZero() {
		t.Errorf("created_at/original_address = %v/%q, want set/mine:follow-ups/01", tk.CreatedAt, tk.OriginalAddress)
	}

	// An idea needs no Evidence, and may name another project.
	idea := "---\ncategory: idea\nstatus: draft\ntype: research\n---\n\n" + followUpSections
	out, err = runIn(t, repo, "tickets", "follow-up", "add", "theirs:follow-ups", "--body", idea, "--json")
	if err != nil {
		t.Fatalf("idea: %v", err)
	}
	if !strings.Contains(out, "theirs:follow-ups/01") {
		t.Errorf("json output %q lacks the address", out)
	}
	tk, err = schema.ParseTicket(cmdtest.AddedPath(t, filepath.Join(theirs, "follow-ups"), "theirs:follow-ups/01"))
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != schema.StatusDraft || tk.Type != schema.TypeResearch || tk.Category != schema.CategoryIdea {
		t.Errorf("status/type/category = %s/%s/%s, want draft/research/idea", tk.Status, tk.Type, tk.Category)
	}
}

func TestExecute_TicketsFollowUpAdd_CategoryFlag(t *testing.T) {
	repo, project := setupShowProject(t)

	out, err := runIn(t, repo, "tickets", "follow-up", "add", "mine:follow-ups", "--category", "idea", "--body", followUpSections)
	if err != nil {
		t.Fatal(err)
	}
	tk, err := schema.ParseTicket(cmdtest.AddedPath(t, filepath.Join(project, "follow-ups"), out))
	if err != nil {
		t.Fatal(err)
	}
	if tk.Category != schema.CategoryIdea {
		t.Errorf("category = %s, want idea", tk.Category)
	}

	both := "---\ncategory: idea\n---\n\n" + followUpSections
	_, err = runIn(t, repo, "tickets", "follow-up", "add", "mine:follow-ups", "--category", "idea", "--body", both)
	var ae *tickets.AddressError
	if !errors.As(err, &ae) || ae.Code != tickets.CodeCategoryConflict {
		t.Fatalf("err = %v, want code %s", err, tickets.CodeCategoryConflict)
	}

	if _, err = runIn(t, repo, "tickets", "follow-up", "add", "mine:follow-ups", "--category", "nope", "--body", followUpSections); err == nil {
		t.Error("invalid --category was accepted")
	}
}

func TestExecute_TicketsFollowUpAdd_Refusals(t *testing.T) {
	repo, project := setupShowProject(t)
	epic := filepath.Join(project, "follow-ups")

	for _, tc := range []struct {
		name, target, body, code, mentions string
	}{
		{"no category", "mine:follow-ups", followUpSections, tickets.CodeCategoryRequired, "category"},
		{"no source", "mine:follow-ups", "---\ncategory: idea\n---\n\n## Problem\n\np\n\n## Suggested fix\n\nf\n", tickets.CodeSectionRequired, "## Source"},
		{"no problem", "mine:follow-ups", "---\ncategory: idea\n---\n\n## Source\n\ns\n\n## Suggested fix\n\nf\n", tickets.CodeSectionRequired, "## Problem"},
		{"no fix", "mine:follow-ups", "---\ncategory: idea\n---\n\n## Source\n\ns\n\n## Problem\n\np\n", tickets.CodeSectionRequired, "## Suggested fix"},
		{"bug without evidence", "mine:follow-ups", "---\ncategory: bug\n---\n\n" + followUpSections, tickets.CodeSectionRequired, "## Evidence"},
		{"other epic", "mine:epic", "---\ncategory: idea\n---\n\n" + followUpSections, tickets.CodeNotFollowUpsEpic, tickets.FollowUpsEpic},
		{"status", "mine:follow-ups", "---\ncategory: idea\nstatus: open\n---\n\n" + followUpSections, tickets.CodeStatusOverride, "open"},
		{"type", "mine:follow-ups", "---\ncategory: idea\ntype: implement\n---\n\n" + followUpSections, tickets.CodeTypeOverride, "implement"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := dirEntries(t, filepath.Join(project, "epic", "issues"))
			_, err := runIn(t, repo, "tickets", "follow-up", "add", tc.target, "--body", tc.body)
			var ae *tickets.AddressError
			if !errors.As(err, &ae) || ae.Code != tc.code {
				t.Fatalf("err = %v, want code %s", err, tc.code)
			}
			if !strings.Contains(ae.Msg, tc.mentions) {
				t.Errorf("message %q does not name %q", ae.Msg, tc.mentions)
			}
			if _, statErr := os.Stat(epic); !os.IsNotExist(statErr) {
				t.Errorf("follow-ups was created: %v", statErr)
			}
			if after := dirEntries(t, filepath.Join(project, "epic", "issues")); after != before {
				t.Errorf("epic issues changed: %q -> %q", before, after)
			}
		})
	}
}

func TestExecute_TicketsFollowUpAdd_RefusesAnOpenDuplicateTitle(t *testing.T) {
	repo, project := setupShowProject(t)
	body := "---\ncategory: idea\n---\n\n# TestSizeguard fails\n\n" + followUpSections
	add := func(title string, extra ...string) (string, error) {
		b := strings.Replace(body, "TestSizeguard fails", title, 1)
		return runIn(t, repo, append([]string{"tickets", "follow-up", "add", "mine:follow-ups", "--body", b}, extra...)...)
	}

	first, err := add("TestSizeguard fails")
	if err != nil {
		t.Fatal(err)
	}
	existing := strings.TrimSpace(first)

	_, err = add("  testsizeguard FAILS ")
	var ae *tickets.AddressError
	if !errors.As(err, &ae) || ae.Code != tickets.CodeDuplicateFollowUp {
		t.Fatalf("err = %v, want code %s", err, tickets.CodeDuplicateFollowUp)
	}
	if !strings.Contains(ae.Msg, existing) || !strings.Contains(ae.Msg, "--force") {
		t.Errorf("message %q does not name %s and --force", ae.Msg, existing)
	}

	// A different title is not a duplicate.
	if _, err := add("TestOther fails"); err != nil {
		t.Fatalf("different title: %v", err)
	}

	// --force files the duplicate.
	out, err := add("TestSizeguard fails", "--force")
	if err != nil {
		t.Fatalf("--force: %v", err)
	}
	if got := strings.TrimSpace(out); got == existing {
		t.Errorf("--force printed the existing address %s", got)
	}

	// A done or cancelled follow-up does not block the title.
	for _, status := range []string{"done", "cancelled"} {
		title := "TestClosed " + status
		out, err := add(title)
		if err != nil {
			t.Fatal(err)
		}
		p := cmdtest.AddedPath(t, filepath.Join(project, "follow-ups"), out)
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(strings.Replace(string(data), "status: draft", "status: "+status, 1)), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := add(title); err != nil {
			t.Errorf("%s follow-up blocked the title: %v", status, err)
		}
	}
}

func TestExecute_TicketsFollowUpAdd_ReportsEveryMissingSection(t *testing.T) {
	repo, _ := setupShowProject(t)
	_, err := runIn(t, repo, "tickets", "follow-up", "add", "mine:follow-ups", "--body", "---\ncategory: bug\n---\n\n## Problem\n\np\n")
	var ae *tickets.AddressError
	if !errors.As(err, &ae) || ae.Code != tickets.CodeSectionRequired {
		t.Fatalf("err = %v, want code %s", err, tickets.CodeSectionRequired)
	}
	for _, h := range []string{"## Source", "## Suggested fix", "## Evidence"} {
		if !strings.Contains(ae.Msg, h) {
			t.Errorf("message %q does not name %q", ae.Msg, h)
		}
	}
	if strings.Contains(ae.Msg, "## Problem") {
		t.Errorf("message %q names the present ## Problem section", ae.Msg)
	}
}

// followUpSearchProject returns a repo whose follow-ups epic holds one draft
// per (id, title, problem) given, and the project dir.
func followUpSearchProject(t *testing.T, drafts ...[3]string) (repo, project string) {
	t.Helper()
	repo = testutil.TempRepo(t)
	project = addProject(t, isolateTicketStore(t), "mine", repo)
	testutil.EnsureEpicTicketMD(t, filepath.Join(project, tickets.FollowUpsEpic))
	issues := filepath.Join(project, tickets.FollowUpsEpic, "issues")
	if err := os.MkdirAll(issues, 0755); err != nil {
		t.Fatal(err)
	}
	for _, d := range drafts {
		testutil.WriteFile(t, issues, d[0]+"-x.md", fmt.Sprintf("---\nid: %q\nstatus: draft\ntype: research\n---\n\n# %s — %s\n\n## Problem\n\n%s\n", d[0], d[0], d[1], d[2]))
	}
	return repo, project
}

func TestExecute_TicketsFollowUpSearch_MatchesOpenAndArchivedWithReason(t *testing.T) {
	repo, project := followUpSearchProject(t, [3]string{"05", "Cleanup", "The Disk fills up."})
	fu := filepath.Join(project, tickets.FollowUpsEpic, "issues")
	testutil.WriteFile(t, fu, "06-menu.md", "---\nid: \"06\"\nstatus: open\ntype: research\n---\n\n# 06 — Menu\n\n## Problem\n\nEscape misbehaves.\n\n## Suggested fix\n\nguard\n")
	if _, err := runIn(t, repo, "tickets", "close", "follow-ups/06", "--reason", "obsolete"); err != nil {
		t.Fatal(err)
	}

	out, err := runIn(t, repo, "tickets", "follow-up", "search", "disk", "escape")
	if err != nil {
		t.Fatal(err)
	}
	want := "mine:follow-ups/05  draft  Cleanup\nmine:follow-ups-archived/01  cancelled  Menu  (obsolete)\n"
	if out != want {
		t.Errorf("search = %q, want %q", out, want)
	}

	// A word only in a non-Problem section does not match.
	out, err = runIn(t, repo, "tickets", "follow-up", "search", "--project", "mine", "guard")
	if err != nil || out != "no matches\n" {
		t.Errorf("search guard = %q, %v; want the no matches line", out, err)
	}
}

func TestExecute_TicketsFollowUpSearch_RepeatedQueryNamesEachMiss(t *testing.T) {
	repo, _ := followUpSearchProject(t, [3]string{"05", "Cleanup", "The Disk fills up."})

	out, err := runIn(t, repo, "tickets", "follow-up", "search", "--query", "disk", "--query", "nothing-like-this", "--query", "also-absent")
	if err != nil {
		t.Fatal(err)
	}
	want := "mine:follow-ups/05  draft  Cleanup\nno matches: nothing-like-this\nno matches: also-absent\n"
	if out != want {
		t.Errorf("search = %q, want %q", out, want)
	}
}

func TestExecute_TicketsFollowUpSearch_CapsArchivedHits(t *testing.T) {
	repo, project := followUpSearchProject(t, [3]string{"01", "Open one", "widget broke."})
	testutil.EnsureEpicTicketMD(t, filepath.Join(project, tickets.ArchivedFollowUpsEpic))
	arch := filepath.Join(project, tickets.ArchivedFollowUpsEpic, "issues")
	if err := os.MkdirAll(arch, 0755); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 12; i++ {
		id := fmt.Sprintf("%02d", i)
		testutil.WriteFile(t, arch, id+"-a.md", fmt.Sprintf("---\nid: %q\nstatus: cancelled\ntype: research\n---\n\n# %s — Old %s\n\n## Problem\n\nwidget broke.\n", id, id, id))
	}

	out, err := runIn(t, repo, "tickets", "follow-up", "search", "widget")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 12 {
		t.Fatalf("got %d lines, want open + 10 archived + more line:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[0], "follow-ups/01") {
		t.Errorf("first line = %q, want the open hit", lines[0])
	}
	if !strings.Contains(lines[1], "follow-ups-archived/12") || !strings.Contains(lines[10], "follow-ups-archived/03") {
		t.Errorf("archived hits not newest first, capped at 10:\n%s", out)
	}
	if last := lines[11]; !strings.Contains(last, "2 more") || !strings.Contains(last, "--all") {
		t.Errorf("last line = %q, want it to count 2 left out and name --all", last)
	}

	out, err = runIn(t, repo, "tickets", "follow-up", "search", "--all", "widget")
	if err != nil {
		t.Fatal(err)
	}
	lines = strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 13 || strings.Contains(out, "more archived") {
		t.Errorf("--all printed %d lines, want open + 12 archived and no more line:\n%s", len(lines), out)
	}
}

func TestExecute_TicketsFollowUpSearch_SplitsArgumentsOnWhitespace(t *testing.T) {
	repo, _ := followUpSearchProject(t, [3]string{"05", "Alpha", "The verify step is slow."}, [3]string{"06", "Beta", "Each ticket leaks."})
	want := "mine:follow-ups/05  draft  Alpha\nmine:follow-ups/06  draft  Beta\n"

	for _, args := range [][]string{{"verify ticket"}, {"verify", "ticket"}, {"  ", "verify  ticket"}} {
		out, err := runIn(t, repo, append([]string{"tickets", "follow-up", "search"}, args...)...)
		if err != nil {
			t.Fatal(err)
		}
		if out != want {
			t.Errorf("search %q = %q, want %q", args, out, want)
		}
	}

	if _, err := runIn(t, repo, "tickets", "follow-up", "search", " ", ""); err == nil || !strings.Contains(err.Error(), "needs at least one word") {
		t.Errorf("blank-only query err = %v, want needs at least one word", err)
	}
}

func dirEntries(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return strings.Join(names, ",")
}
