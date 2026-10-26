package server_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

// startDeletable seeds proj with: bare (uncharted map), charted (map with a
// Destination), ticketed (map with a ticket), live (uncharted map with a
// session) and plain (a loop epic).
func startDeletable(t *testing.T) designsFixture {
	t.Helper()
	store := t.TempDir()
	mapMD := func(epic, body string) {
		dir := filepath.Join(store, "proj", epic)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "ticket.md"), []byte("---\nkind: map\n---\n"+body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mapMD("bare", "\n## Initial prompt\n\nidea\n")
	mapMD("charted", "\n## Destination\n\nsomewhere\n")
	mapMD("live", "\n## Initial prompt\n\nidea\n")
	servertest.WriteTicket(t, store, "proj", "ticketed", "01", "first", "")
	mapMD("ticketed", "\n## Initial prompt\n\nidea\n")
	servertest.WriteTicket(t, store, "proj", "plain", "01", "first", "")
	return startDesignsIn(t, store)
}

func (f designsFixture) del(t *testing.T, epic string) server.SessionResult {
	t.Helper()
	res, err := f.h.Client.DeleteDesign(f.ctx, server.DesignDeleteRequest{Epic: epic})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestDesignsDelete_DeletesAnUnchartedDesign(t *testing.T) {
	f := startDeletable(t)
	if res := f.del(t, "proj:bare"); res.Refused {
		t.Fatalf("delete = %+v", res)
	}
	if _, err := os.Stat(filepath.Join(f.h.TicketStore, "proj", "bare")); !os.IsNotExist(err) {
		t.Errorf("epic dir still there: %v", err)
	}
}

func TestDesignsDelete_RemovesTheDesignWorktreeAndBranch(t *testing.T) {
	f := startDeletable(t)
	res := f.open(t, "proj:bare", false)
	if res.Refused || res.Session == nil {
		t.Fatalf("open = %+v", res)
	}
	dir, _ := readMeta(t, f.h, res.Session.ID)["dir"].(string)
	if err := os.WriteFile(filepath.Join(dir, "design.md"), []byte("design\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitOut(t, dir, "add", "design.md")
	gitOut(t, dir, "commit", "-m", "design work")
	if err := os.WriteFile(filepath.Join(dir, "uncommitted.md"), []byte("wip\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.stop(t, res.Session.ID)
	waitFor(t, func() bool { return len(f.live(t)) == 0 })

	if r := f.del(t, "proj:bare"); r.Refused {
		t.Fatalf("delete = %+v", r)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("worktree still there: %v", err)
	}
	if out := gitOut(t, f.repo, "branch", "--list", "bare-design"); out != "" {
		t.Errorf("branch still there: %q", out)
	}
	if _, err := os.Stat(filepath.Join(f.h.TicketStore, "proj", "bare")); !os.IsNotExist(err) {
		t.Errorf("epic dir still there: %v", err)
	}
}

func TestDesignsDelete_Refusals(t *testing.T) {
	f := startDeletable(t)
	if res := f.open(t, "proj:live", false); res.Session == nil {
		t.Fatalf("open = %+v", res)
	}
	for epic, want := range map[string]string{
		"proj:nope":     server.ReasonDesignUnknownEpic,
		"proj:plain":    server.ReasonDesignNotMapEpic,
		"proj:charted":  server.ReasonDesignNotUncharted,
		"proj:ticketed": server.ReasonDesignNotUncharted,
		"proj:live":     server.ReasonDesignNotUncharted,
	} {
		if res := f.del(t, epic); !res.Refused || res.Reason != want {
			t.Errorf("delete %s = %+v, want %s", epic, res, want)
		}
		if _, err := os.Stat(filepath.Join(f.h.TicketStore, "proj", epic[len("proj:"):])); epic != "proj:nope" && err != nil {
			t.Errorf("delete %s removed the epic: %v", epic, err)
		}
	}
}

func TestDesignsDelete_EpicDetailDeletableMatchesTheRule(t *testing.T) {
	f := startDeletable(t)
	f.open(t, "proj:live", false)
	for epic, want := range map[string]bool{"proj:bare": true, "proj:charted": false, "proj:ticketed": false, "proj:live": false} {
		d, err := f.h.Client.EpicDetail(f.ctx, epic)
		if err != nil {
			t.Fatal(err)
		}
		if d.Deletable != want {
			t.Errorf("%s deletable = %v, want %v", epic, d.Deletable, want)
		}
	}
}

func TestDesignsDelete_EpicDetailLogsADeletableCheckError(t *testing.T) {
	f := startDeletable(t)
	// A file where the sessions dir should be makes listing sessions fail.
	if err := os.RemoveAll(f.h.SessionsDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.h.SessionsDir, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := f.h.Client.EpicDetail(f.ctx, "proj:bare")
	if err != nil {
		t.Fatal(err)
	}
	if d.Deletable {
		t.Error("deletable = true after the check failed")
	}
	log, err := os.ReadFile(server.LogPath(f.h.StateDir))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), "design deletable check") {
		t.Errorf("error not logged:\n%s", log)
	}
}

func TestDesignsDelete_SlugIsFreeAgain(t *testing.T) {
	f := startDeletable(t)
	if res := f.del(t, "proj:bare"); res.Refused {
		t.Fatalf("delete = %+v", res)
	}
	res, err := f.h.Client.OpenDesign(f.ctx, server.DesignOpenRequest{Epic: "bare", NewDesign: true, Idea: "again", Project: "proj"})
	if err != nil || res.Refused || res.Session == nil {
		t.Fatalf("new design = %+v, %v", res, err)
	}
}
