package tickets

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

// TestModel_ScratchDirResolvesThroughStore asserts scratchDir() is the repo's
// project directory in the ticket store. Not parallel-safe (sets env).
func TestModel_ScratchDirResolvesThroughStore(t *testing.T) {
	root := testutil.TempRepo(t)
	project := addStoreProject(t, root)
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}

	m := NewModel(sub, ui.Settings{}, keys.New(nil))
	if got := m.scratchDir(); got != project {
		t.Errorf("scratchDir() = %q, want %q (store project)", got, project)
	}
}

// TestScratchRoot_CallSitesAgreeAcrossWorktreesInBareRepo verifies the
// Tickets tab (Model), Queue tab (QueueModel), and implement run options all
// resolve the same canonical `.scratch` regardless of which linked worktree
// of a bare-repo checkout they're scoped to.
func TestScratchRoot_CallSitesAgreeAcrossWorktreesInBareRepo(t *testing.T) {
	outer := testutil.TempDotBareRepoWithWorktrees(t, "feature", "other")
	featureWt := filepath.Join(outer, "feature")
	otherWt := filepath.Join(outer, "other")

	info, err := git.IdentifyDir(featureWt)
	if err != nil {
		t.Fatalf("IdentifyDir: %v", err)
	}
	wantScratchRoot := addStoreProject(t, info.Repo.Root)

	ticketPath := filepath.Join(wantScratchRoot, "alpha", "issues", "01-first.md")
	if err := os.MkdirAll(filepath.Dir(ticketPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ticketPath, []byte(LegacyTicketToFrontmatter("01-first.md", "Status: open\n\nBody.\n")), 0644); err != nil {
		t.Fatal(err)
	}

	for _, wt := range []string{featureWt, otherWt} {
		m := deliverLoad(t, NewModel(wt, ui.Settings{}, keys.New(nil)))
		if got := m.scratchDir(); got != wantScratchRoot {
			t.Errorf("Model.scratchDir() from %q = %q, want %q", wt, got, wantScratchRoot)
		}
		if len(m.epics) != 1 {
			t.Fatalf("Model from %q: expected 1 epic loaded from shared .scratch, got %d", wt, len(m.epics))
		}

		qm := loadQueueModel(t, NewQueueModel(wt, ui.Settings{}, map[string]bool{}, keys.Manager{}))
		if len(qm.epics) != 1 {
			t.Fatalf("QueueModel from %q: expected 1 epic loaded from shared .scratch, got %d", wt, len(qm.epics))
		}

		opts, err := buildImplementRunOptions(wt, "alpha", ralphloop.AgentClaude)
		if err != nil {
			t.Fatalf("buildImplementRunOptions from %q: %v", wt, err)
		}
		if opts.ScratchDir != wantScratchRoot {
			t.Errorf("buildImplementRunOptions from %q: ScratchDir = %q, want %q", wt, opts.ScratchDir, wantScratchRoot)
		}
	}
}
