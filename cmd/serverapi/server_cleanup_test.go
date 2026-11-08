package serverapi

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

func TestServerCleanup_JSONPrintsTheScan(t *testing.T) {
	store := t.TempDir()
	repo, wt := testutil.TempRepoWithLinkedWorktree(t, "feature-x")
	if err := servertest.WriteEpic(store, "proj", ""); err != nil {
		t.Fatal(err)
	}
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store)

	var out bytes.Buffer
	if err := runServerCleanup(context.Background(), h.Client, &out, "proj", true); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Project   string `json:"project"`
		Worktrees []struct {
			Path string `json:"path"`
		} `json:"worktrees"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range got.Worktrees {
		found = found || w.Path == wt
	}
	if got.Project != "proj" || !found {
		t.Errorf("--json = %s", out.String())
	}
}
