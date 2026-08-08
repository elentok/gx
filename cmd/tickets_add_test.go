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
	if err := runTicketsAdd(epicPath, "", &stdout); err != nil {
		t.Fatalf("runTicketsAdd: %v", err)
	}

	wantPath := filepath.Join(issuesDir, "04-new-ticket.md")
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

func TestRunTicketsAdd_LetteredChild(t *testing.T) {
	scratchDir := t.TempDir()
	epicPath := filepath.Join(scratchDir, "widget-epic")
	issuesDir := filepath.Join(epicPath, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("mkdir issues: %v", err)
	}
	writeTicket(t, filepath.Join(issuesDir, "12-parent.md"), "12", "done", "task")

	var stdout bytes.Buffer
	if err := runTicketsAdd(epicPath, "12", &stdout); err != nil {
		t.Fatalf("runTicketsAdd: %v", err)
	}
	firstPath := strings.TrimSpace(stdout.String())
	if firstPath != filepath.Join(issuesDir, "12a-new-ticket.md") {
		t.Fatalf("stdout = %q, want %q", firstPath, filepath.Join(issuesDir, "12a-new-ticket.md"))
	}

	stdout.Reset()
	if err := runTicketsAdd(epicPath, "12", &stdout); err != nil {
		t.Fatalf("runTicketsAdd (second call): %v", err)
	}
	secondPath := strings.TrimSpace(stdout.String())
	if secondPath != filepath.Join(issuesDir, "12b-new-ticket.md") {
		t.Fatalf("stdout = %q, want %q", secondPath, filepath.Join(issuesDir, "12b-new-ticket.md"))
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
	if err := runTicketsAdd(epicPath, "12b", &stdout); err != nil {
		t.Fatalf("runTicketsAdd: %v", err)
	}
	firstPath := strings.TrimSpace(stdout.String())
	if firstPath != filepath.Join(issuesDir, "12b1-new-ticket.md") {
		t.Fatalf("stdout = %q, want %q", firstPath, filepath.Join(issuesDir, "12b1-new-ticket.md"))
	}

	stdout.Reset()
	if err := runTicketsAdd(epicPath, "12b", &stdout); err != nil {
		t.Fatalf("runTicketsAdd (second call): %v", err)
	}
	secondPath := strings.TrimSpace(stdout.String())
	if secondPath != filepath.Join(issuesDir, "12b2-new-ticket.md") {
		t.Fatalf("stdout = %q, want %q", secondPath, filepath.Join(issuesDir, "12b2-new-ticket.md"))
	}
}

func TestRunTicketsAdd_RejectsNestingPastLetteredNumericParent(t *testing.T) {
	scratchDir := t.TempDir()
	epicPath := filepath.Join(scratchDir, "widget-epic")
	issuesDir := filepath.Join(epicPath, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("mkdir issues: %v", err)
	}

	var stdout bytes.Buffer
	err := runTicketsAdd(epicPath, "12b1", &stdout)
	if err == nil {
		t.Fatal("expected error for --parent=12b1, got nil")
	}
	entries, readErr := os.ReadDir(issuesDir)
	if readErr != nil {
		t.Fatalf("read issues dir: %v", readErr)
	}
	if len(entries) != 0 {
		t.Errorf("issues dir has %d entries, want 0 (no file created)", len(entries))
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
			errs[i] = runTicketsAdd(epicPath, "", &buf)
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
