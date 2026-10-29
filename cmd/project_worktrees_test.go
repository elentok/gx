package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

func TestProjectWorktrees_PrintsRootAndWorktree(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	repo, wt := testutil.TempRepoWithLinkedWorktree(t, "feature")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store)

	list, err := h.Client.ProjectWorktrees(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := printProjectWorktrees(&out, list); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "proj\t") || !strings.HasPrefix(lines[1], "proj\t") {
		t.Fatalf("output = %q, want a root line and a worktree line", out.String())
	}
	if !strings.HasSuffix(lines[1], "feature") || !strings.Contains(wt, "feature") {
		t.Errorf("worktree line = %q", lines[1])
	}
}
