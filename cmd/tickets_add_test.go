package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/elentok/gx/tickets/schema"
)

func TestRunTicketsAdd_FlatSibling(t *testing.T) {
	scratchDir := t.TempDir()
	epicPath := filepath.Join(scratchDir, "widget-epic")
	issuesDir := filepath.Join(epicPath, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("mkdir issues: %v", err)
	}
	writeTicket(t, filepath.Join(issuesDir, "01-do-thing.md"), "01", "done", "task")
	writeTicket(t, filepath.Join(issuesDir, "02-do-other-thing.md"), "02", "done", "task")
	writeTicket(t, filepath.Join(issuesDir, "03-do-third-thing.md"), "03", "done", "task")

	var stdout bytes.Buffer
	if err := runTicketsAdd(epicPath, "", "do-fourth-thing", &stdout); err != nil {
		t.Fatalf("runTicketsAdd: %v", err)
	}

	wantPath := filepath.Join(issuesDir, "04-do-fourth-thing.md")
	gotPath := strings.TrimSpace(stdout.String())
	if gotPath != wantPath {
		t.Fatalf("stdout = %q, want %q", gotPath, wantPath)
	}
	ticket, err := schema.ParseTicket(wantPath)
	if err != nil {
		t.Fatalf("stub ticket failed validation: %v", err)
	}
	if ticket.ID != "04" {
		t.Errorf("stub ticket id = %q, want %q", ticket.ID, "04")
	}
}

func TestRunTicketsAdd_WritesStatusDraft(t *testing.T) {
	scratchDir := t.TempDir()
	epicPath := filepath.Join(scratchDir, "widget-epic")
	issuesDir := filepath.Join(epicPath, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("mkdir issues: %v", err)
	}

	var stdout bytes.Buffer
	if err := runTicketsAdd(epicPath, "", "do-thing", &stdout); err != nil {
		t.Fatalf("runTicketsAdd: %v", err)
	}

	gotPath := strings.TrimSpace(stdout.String())
	ticket, err := schema.ParseTicket(gotPath)
	if err != nil {
		t.Fatalf("stub ticket failed validation: %v", err)
	}
	if ticket.Status != schema.StatusDraft {
		t.Errorf("stub ticket status = %q, want %q", ticket.Status, schema.StatusDraft)
	}
}

func TestRunTicketsAdd_EmptySlugFails(t *testing.T) {
	scratchDir := t.TempDir()
	epicPath := filepath.Join(scratchDir, "widget-epic")
	issuesDir := filepath.Join(epicPath, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("mkdir issues: %v", err)
	}

	var stdout bytes.Buffer
	err := runTicketsAdd(epicPath, "", "", &stdout)
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
	scratchDir := t.TempDir()
	epicPath := filepath.Join(scratchDir, "widget-epic")
	issuesDir := filepath.Join(epicPath, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("mkdir issues: %v", err)
	}
	writeTicket(t, filepath.Join(issuesDir, "12-parent.md"), "12", "done", "task")

	var stdout bytes.Buffer
	if err := runTicketsAdd(epicPath, "12", "child-a", &stdout); err != nil {
		t.Fatalf("runTicketsAdd: %v", err)
	}
	firstPath := strings.TrimSpace(stdout.String())
	if firstPath != filepath.Join(issuesDir, "12a-child-a.md") {
		t.Fatalf("stdout = %q, want %q", firstPath, filepath.Join(issuesDir, "12a-child-a.md"))
	}

	stdout.Reset()
	if err := runTicketsAdd(epicPath, "12", "child-b", &stdout); err != nil {
		t.Fatalf("runTicketsAdd (second call): %v", err)
	}
	secondPath := strings.TrimSpace(stdout.String())
	if secondPath != filepath.Join(issuesDir, "12b-child-b.md") {
		t.Fatalf("stdout = %q, want %q", secondPath, filepath.Join(issuesDir, "12b-child-b.md"))
	}
}

func TestRunTicketsAdd_WritesParentFrontmatter(t *testing.T) {
	scratchDir := t.TempDir()
	epicPath := filepath.Join(scratchDir, "widget-epic")
	issuesDir := filepath.Join(epicPath, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("mkdir issues: %v", err)
	}
	writeTicket(t, filepath.Join(issuesDir, "12-parent.md"), "12", "done", "task")

	var stdout bytes.Buffer
	if err := runTicketsAdd(epicPath, "12", "child-a", &stdout); err != nil {
		t.Fatalf("runTicketsAdd: %v", err)
	}
	gotPath := strings.TrimSpace(stdout.String())

	ticket, err := schema.ParseTicket(gotPath)
	if err != nil {
		t.Fatalf("stub ticket failed validation: %v", err)
	}
	if ticket.Parent == nil || *ticket.Parent != "12" {
		t.Errorf("stub ticket parent = %v, want \"12\"", ticket.Parent)
	}
}

func TestRunTicketsAdd_BackfillsParentChildren(t *testing.T) {
	scratchDir := t.TempDir()
	epicPath := filepath.Join(scratchDir, "widget-epic")
	issuesDir := filepath.Join(epicPath, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("mkdir issues: %v", err)
	}
	writeTicket(t, filepath.Join(issuesDir, "12-parent.md"), "12", "done", "task")

	var stdout bytes.Buffer
	if err := runTicketsAdd(epicPath, "12", "child-a", &stdout); err != nil {
		t.Fatalf("runTicketsAdd: %v", err)
	}

	parent, err := schema.ParseTicket(filepath.Join(issuesDir, "12-parent.md"))
	if err != nil {
		t.Fatalf("parsing parent ticket: %v", err)
	}
	if len(parent.Children) != 1 || parent.Children[0] != "12a" {
		t.Fatalf("parent children = %v, want [12a]", parent.Children)
	}

	stdout.Reset()
	if err := runTicketsAdd(epicPath, "12", "child-b", &stdout); err != nil {
		t.Fatalf("runTicketsAdd (second call): %v", err)
	}
	parent, err = schema.ParseTicket(filepath.Join(issuesDir, "12-parent.md"))
	if err != nil {
		t.Fatalf("parsing parent ticket: %v", err)
	}
	if len(parent.Children) != 2 || parent.Children[0] != "12a" || parent.Children[1] != "12b" {
		t.Fatalf("parent children = %v, want [12a 12b]", parent.Children)
	}
}

func TestRunTicketsAdd_NumericLevelPastLetteredParent(t *testing.T) {
	scratchDir := t.TempDir()
	epicPath := filepath.Join(scratchDir, "widget-epic")
	issuesDir := filepath.Join(epicPath, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("mkdir issues: %v", err)
	}
	writeTicket(t, filepath.Join(issuesDir, "12b-child.md"), "12b", "done", "task")

	var stdout bytes.Buffer
	if err := runTicketsAdd(epicPath, "12b", "grandchild-1", &stdout); err != nil {
		t.Fatalf("runTicketsAdd: %v", err)
	}
	firstPath := strings.TrimSpace(stdout.String())
	if firstPath != filepath.Join(issuesDir, "12b1-grandchild-1.md") {
		t.Fatalf("stdout = %q, want %q", firstPath, filepath.Join(issuesDir, "12b1-grandchild-1.md"))
	}

	stdout.Reset()
	if err := runTicketsAdd(epicPath, "12b", "grandchild-2", &stdout); err != nil {
		t.Fatalf("runTicketsAdd (second call): %v", err)
	}
	secondPath := strings.TrimSpace(stdout.String())
	if secondPath != filepath.Join(issuesDir, "12b2-grandchild-2.md") {
		t.Fatalf("stdout = %q, want %q", secondPath, filepath.Join(issuesDir, "12b2-grandchild-2.md"))
	}
}

func TestRunTicketsAdd_LetteredNumericParentAllocatesNextSibling(t *testing.T) {
	scratchDir := t.TempDir()
	epicPath := filepath.Join(scratchDir, "widget-epic")
	issuesDir := filepath.Join(epicPath, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("mkdir issues: %v", err)
	}
	writeTicket(t, filepath.Join(issuesDir, "12b1-child.md"), "12b1", "done", "task")

	var stdout bytes.Buffer
	if err := runTicketsAdd(epicPath, "12b1", "sibling", &stdout); err != nil {
		t.Fatalf("runTicketsAdd(parent=12b1): %v", err)
	}
	gotPath := strings.TrimSpace(stdout.String())
	if gotPath != filepath.Join(issuesDir, "12b2-sibling.md") {
		t.Fatalf("stdout = %q, want %q", gotPath, filepath.Join(issuesDir, "12b2-sibling.md"))
	}
}

func TestRunTicketsAdd_ConcurrentCallsAllocateDistinctIDs(t *testing.T) {
	scratchDir := t.TempDir()
	epicPath := filepath.Join(scratchDir, "widget-epic")
	issuesDir := filepath.Join(epicPath, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("mkdir issues: %v", err)
	}

	const n = 15
	paths := make([]string, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var buf bytes.Buffer
			errs[i] = runTicketsAdd(epicPath, "", "concurrent", &buf)
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
