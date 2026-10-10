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
