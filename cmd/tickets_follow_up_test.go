package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	tk, err := schema.ParseTicket(addedPath(t, filepath.Join(project, "follow-ups"), out))
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != schema.StatusDraft || tk.Type != schema.TypeResearch || tk.Category != schema.CategoryBug {
		t.Errorf("status/type/category = %s/%s/%s, want draft/research/bug", tk.Status, tk.Type, tk.Category)
	}
	if tk.OriginalAddress != "mine:follow-ups/01" || tk.CreatedAt.IsZero() {
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
	tk, err = schema.ParseTicket(addedPath(t, filepath.Join(theirs, "follow-ups"), "theirs:follow-ups/01"))
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
	tk, err := schema.ParseTicket(addedPath(t, filepath.Join(project, "follow-ups"), out))
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
		{"other epic", "mine:epic", "---\ncategory: idea\n---\n\n" + followUpSections, tickets.CodeNotFollowUpsEpic, followUpsEpic},
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

func TestExecute_TicketsFollowUpAdd_HelpListsRequiredSections(t *testing.T) {
	repo, _ := setupShowProject(t)
	out, err := runIn(t, repo, "tickets", "follow-up", "add", "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"Source", "Problem", "Suggested fix", "Evidence"} {
		if !strings.Contains(out, h) {
			t.Errorf("--help does not list %q:\n%s", h, out)
		}
	}
}

func TestExecute_TicketsFollowUpSearch_MatchesOpenAndArchivedWithReason(t *testing.T) {
	f := setupCloseProject(t)
	fu := filepath.Join(f.project, followUpsEpic, "issues")
	testutil.WriteFile(t, fu, "05-disk.md", "---\nid: \"05\"\nstatus: draft\ntype: research\n---\n\n# 05 — Cleanup\n\n## Problem\n\nThe Disk fills up.\n")
	testutil.WriteFile(t, fu, "06-menu.md", "---\nid: \"06\"\nstatus: open\ntype: research\n---\n\n# 06 — Menu\n\n## Problem\n\nEscape misbehaves.\n\n## Suggested fix\n\nguard\n")
	if _, err := runIn(t, f.repo, "tickets", "close", "follow-ups/06", "--reason", "obsolete"); err != nil {
		t.Fatal(err)
	}

	out, err := runIn(t, f.repo, "tickets", "follow-up", "search", "disk", "escape")
	if err != nil {
		t.Fatal(err)
	}
	want := "mine:follow-ups/05  draft  Cleanup\nmine:follow-ups-archived/01  cancelled  Menu  (obsolete)\n"
	if out != want {
		t.Errorf("search = %q, want %q", out, want)
	}

	// A word only in a non-Problem section does not match.
	out, err = runIn(t, f.repo, "tickets", "follow-up", "search", "--project", "mine", "guard")
	if err != nil || out != "" {
		t.Errorf("search guard = %q, %v; want no hits", out, err)
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
