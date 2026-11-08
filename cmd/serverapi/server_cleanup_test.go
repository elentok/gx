package serverapi

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
)

func TestServerCleanupWrite_JSONPrintsAndARefusalExitsNonZero(t *testing.T) {
	store := t.TempDir()
	repo := testutil.TempRepo(t)
	if err := servertest.WriteEpic(store, "proj", ""); err != nil {
		t.Fatal(err)
	}
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store)
	call := (*apiclient.Reads).DeleteCleanupBranch

	var out bytes.Buffer
	err := runServerCleanupWrite(context.Background(), h.Client, &out, call, wire.CleanupWriteRequest{Project: "proj", Branch: "nope"}, true)
	if err == nil || !strings.Contains(out.String(), wire.ReasonCleanupUnknown) {
		t.Errorf("err = %v, out = %s", err, out.String())
	}
}

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
