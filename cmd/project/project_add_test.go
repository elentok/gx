package project

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
)

func TestProjectAdd_RegistersAndListsAndRefuses(t *testing.T) {
	t.Parallel()
	h := servertest.Start(t)
	ctx := context.Background()
	repo := testutil.TempRepo(t)
	notRepo := t.TempDir()

	add := func(req wire.AddProjectRequest) (string, error) {
		var out bytes.Buffer
		err := runProjectAdd(ctx, h.Client, Deps{}, &out, false, req)
		return out.String(), err
	}

	out, err := add(wire.AddProjectRequest{Path: repo})
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Base(repo)
	if want := "added project " + name + "\n"; !strings.HasPrefix(out, want) {
		t.Errorf("add output = %q, want prefix %q", out, want)
	}

	pf, err := config.ReadProjectFile(filepath.Join(h.TicketStore, name))
	if err != nil || pf.Repo == nil {
		t.Fatalf("project.json = %+v, %v", pf, err)
	}

	list, err := h.Client.Reads.Projects(ctx)
	if err != nil || len(list) != 2 || !slices.ContainsFunc(list, func(p wire.ProjectInfo) bool { return p.Name == name }) {
		t.Fatalf("list = %+v, %v", list, err)
	}

	// Re-adding the same path (even under another name) names the existing project.
	out, err = add(wire.AddProjectRequest{Path: repo, Name: "other"})
	if err != nil || !strings.Contains(out, "already registered as project "+name) {
		t.Errorf("re-add = %q, %v", out, err)
	}

	for _, tc := range []struct {
		desc, reason string
		req          wire.AddProjectRequest
	}{
		{"scratch reserved", wire.ReasonReservedName, wire.AddProjectRequest{Path: notRepo, Name: "scratch", VCS: "none"}},
		{"duplicate name", wire.ReasonNameTaken, wire.AddProjectRequest{Path: notRepo, Name: name, VCS: "none"}},
		{"non-repo", wire.ReasonNotRepo, wire.AddProjectRequest{Path: notRepo}},
		{"bad name", wire.ReasonBadName, wire.AddProjectRequest{Path: notRepo, Name: "a/b", VCS: "none"}},
	} {
		if _, err := add(tc.req); err == nil || !strings.Contains(err.Error(), tc.reason) {
			t.Errorf("%s: err = %v, want reason %s", tc.desc, err, tc.reason)
		}
	}

	// A non-repo registers with --vcs none.
	if _, err := add(wire.AddProjectRequest{Path: notRepo, Name: "loose", VCS: "none"}); err != nil {
		t.Errorf("vcs none: %v", err)
	}
}

func TestProjectAdd_DefaultNameIsDirHoldingBare(t *testing.T) {
	h := servertest.Start(t)
	bare := testutil.TempDotBareRepoWithWorktrees(t)
	var out bytes.Buffer
	if err := runProjectAdd(context.Background(), h.Client, Deps{}, &out, false, wire.AddProjectRequest{Path: bare}); err != nil {
		t.Fatal(err)
	}
	want := filepath.Base(bare)
	if _, err := os.Stat(filepath.Join(h.TicketStore, want, config.ProjectFileName)); err != nil {
		t.Errorf("project %q not registered: %v (output %q)", want, err, out.String())
	}
}
