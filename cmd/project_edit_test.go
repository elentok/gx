package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
)

func TestProjectRemove_RefusesWhileQueuedKeepsTickets(t *testing.T) {
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic", "01", "a", "")
	h := servertest.StartWithStore(t, store)
	ctx := context.Background()
	remove := func() error {
		return runProjectEdit(ctx, &bytes.Buffer{}, false, "removed", h.Client.RemoveProject, server.ProjectRequest{Name: "proj"})
	}

	if q, err := h.Client.QueueAdd(ctx, "proj:epic/01", ""); err != nil || q.Refused {
		t.Fatalf("queue add: %+v, %v", q, err)
	}
	if err := remove(); err == nil || !strings.Contains(err.Error(), server.ReasonProjectBusy) {
		t.Fatalf("remove while queued: err = %v", err)
	}
	if _, err := h.Client.QueueRemove(ctx, "proj:epic/01"); err != nil {
		t.Fatal(err)
	}
	if err := remove(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store, "proj", "epic", "issues", "01-a.md")); err != nil {
		t.Errorf("ticket gone after remove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store, "proj", config.ProjectFileName)); !os.IsNotExist(err) {
		t.Errorf("project.json still there: %v", err)
	}
}

func TestProjectSetPath_ClearsVCSNoneOnlyForGitRepo(t *testing.T) {
	h := servertest.Start(t)
	ctx := context.Background()
	outbox, repo := t.TempDir(), testutil.TempRepo(t)
	if err := runProjectAdd(ctx, h.Client, &bytes.Buffer{}, false, server.AddProjectRequest{Path: outbox, Name: "ob", VCS: "none"}); err != nil {
		t.Fatal(err)
	}
	setPath := func(path string) {
		t.Helper()
		if err := runProjectEdit(ctx, &bytes.Buffer{}, false, "updated", h.Client.SetProjectPath, server.ProjectRequest{Name: "ob", Path: path}); err != nil {
			t.Fatal(err)
		}
	}
	vcs := func() *string {
		t.Helper()
		pf, err := config.ReadProjectFile(filepath.Join(h.TicketStore, "ob"))
		if err != nil {
			t.Fatal(err)
		}
		return pf.VCS
	}

	setPath(t.TempDir())
	if v := vcs(); v == nil || *v != config.VCSNone {
		t.Errorf("vcs after non-git set-path = %v, want none", v)
	}
	setPath(repo)
	if v := vcs(); v != nil {
		t.Errorf("vcs after git set-path = %q, want cleared", *v)
	}

	// The clash hint only applies to a ticket-only project.
	err := runProjectAdd(ctx, h.Client, &bytes.Buffer{}, false, server.AddProjectRequest{Path: outbox, Name: "ob", VCS: "none"})
	if err == nil || strings.Contains(err.Error(), "gx project set-path") {
		t.Errorf("clash with a git project: err = %v, want no set-path hint", err)
	}
}

func TestProjectAdd_ClashWithVCSNoneProjectHintsSetPath(t *testing.T) {
	h := servertest.Start(t)
	ctx := context.Background()
	if err := runProjectAdd(ctx, h.Client, &bytes.Buffer{}, false, server.AddProjectRequest{Path: t.TempDir(), Name: "ob", VCS: "none"}); err != nil {
		t.Fatal(err)
	}
	err := runProjectAdd(ctx, h.Client, &bytes.Buffer{}, false, server.AddProjectRequest{Path: testutil.TempRepo(t), Name: "ob"})
	if err == nil || !strings.Contains(err.Error(), "gx project set-path ob <clone>") {
		t.Errorf("err = %v, want set-path hint", err)
	}
}

func TestProjectSetPath_ChangesResolvedPath(t *testing.T) {
	h := servertest.Start(t)
	ctx := context.Background()
	a, b := testutil.TempRepo(t), testutil.TempRepo(t)
	name := filepath.Base(a)
	if err := runProjectAdd(ctx, h.Client, &bytes.Buffer{}, false, server.AddProjectRequest{Path: a}); err != nil {
		t.Fatal(err)
	}
	if err := runProjectEdit(ctx, &bytes.Buffer{}, false, "updated", h.Client.SetProjectPath, server.ProjectRequest{Name: name, Path: b}); err != nil {
		t.Fatal(err)
	}
	pf, err := config.ReadProjectFile(filepath.Join(h.TicketStore, name))
	if err != nil || pf.Repo == nil || *pf.Repo == "" || filepath.Base(*pf.Repo) != filepath.Base(b) {
		t.Errorf("project.json = %+v, %v; want repo %s", pf, err, b)
	}
	err = runProjectEdit(ctx, &bytes.Buffer{}, false, "updated", h.Client.SetProjectPath, server.ProjectRequest{Name: "nope", Path: b})
	if err == nil || !strings.Contains(err.Error(), server.ReasonUnknownProject) {
		t.Errorf("unknown project: err = %v", err)
	}
}
