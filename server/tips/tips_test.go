package tips_test

import (
	"context"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
)

func tipsFor(t *testing.T, goos string, extra ...wire.Tip) ([]wire.Tip, string) {
	t.Helper()
	store := t.TempDir()
	repo := testutil.TempRepo(t)
	if err := servertest.WriteEpic(store, "proj", ""); err != nil {
		t.Fatal(err)
	}
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, func(c *server.Config) {
		c.TipsGOOS = goos
		c.TipsExtra = extra
	})
	got, err := h.Client.Reads.Tips(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return got, repo
}

func TestTips_MacOSListsSpotlightWithTheWorktreeFolder(t *testing.T) {
	got, repo := tipsFor(t, "darwin")
	if len(got) != 1 || got[0].ID != "spotlight" {
		t.Fatalf("tips = %+v", got)
	}
	want := repo + "/.worktrees"
	if len(got[0].Folders) != 1 || got[0].Folders[0] != want {
		t.Errorf("folders = %v, want [%s]", got[0].Folders, want)
	}
}

func TestTips_OtherOSIsEmpty(t *testing.T) {
	got, _ := tipsFor(t, "linux")
	if got == nil || len(got) != 0 {
		t.Errorf("tips = %#v, want an empty list", got)
	}
}

func TestTips_AnExtraListEntryIsServed(t *testing.T) {
	got, _ := tipsFor(t, "linux", wire.Tip{ID: "fake", Title: "Fake", Platform: "linux"})
	if len(got) != 1 || got[0].ID != "fake" {
		t.Errorf("tips = %+v", got)
	}
}
