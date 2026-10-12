package server

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

// rootRefsFixture is a repo with trunk develop and branches a (a stacked
// blocker's feature branch) and release-1.2, under an on-done project.
func rootRefsFixture(t *testing.T) (*Server, string) {
	t.Helper()
	repo := testutil.TempRepo(t)
	for _, b := range []string{"develop", "a", "release-1.2"} {
		runGit(t, repo, "branch", b)
	}
	runGit(t, repo, "checkout", "-q", "a")
	runGit(t, repo, "commit", "-q", "--allow-empty", "-m", "blocker work")
	runGit(t, repo, "checkout", "-q", "main")
	store := t.TempDir()
	if err := os.MkdirAll(filepath.Join(store, "proj"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"name":"proj","repo":"` + repo + `","trunk":"develop","landing":"on-done"}`
	if err := os.WriteFile(filepath.Join(store, "proj", "project.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return &Server{cfg: Config{TicketStore: store}, log: slog.New(slog.NewTextHandler(io.Discard, nil))}, repo
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestRootRefs(t *testing.T) {
	stacked := func(base string) []tickets.Epic {
		return []tickets.Epic{
			{Name: "a", Tickets: []tickets.Ticket{{Number: 1, Identifier: "01", Status: "done"}}},
			{Name: "b", BlockedBy: []string{"a/01"}, Tickets: []tickets.Ticket{{Number: 1, Identifier: "01", Status: "open", Base: base}}},
		}
	}
	for name, tc := range map[string]struct {
		epics              []tickets.Epic
		landBlocker        bool
		wantBase, wantLand string
	}{
		"no base starts from and lands on trunk": {
			epics: []tickets.Epic{{Name: "b", Tickets: []tickets.Ticket{{Number: 1, Identifier: "01", Status: "open"}}}}, wantBase: "develop", wantLand: "develop",
		},
		"commitless ref is trunk": {
			epics: []tickets.Epic{{Name: "b", Tickets: []tickets.Ticket{{Number: 1, Identifier: "01", Status: "open", Commitless: true}}}}, wantBase: "develop", wantLand: "develop",
		},
		"explicit base moves only the start":             {epics: stacked("release-1.2"), wantBase: "release-1.2", wantLand: "develop"},
		"stacked root targets its unlanded blocker":      {epics: stacked(""), wantBase: "a", wantLand: "a"},
		"stacked root targets trunk once blocker landed": {epics: stacked(""), landBlocker: true, wantBase: "develop", wantLand: "develop"},
	} {
		t.Run(name, func(t *testing.T) {
			s, repo := rootRefsFixture(t)
			if tc.landBlocker {
				runGit(t, repo, "branch", "-f", "develop", "a")
			}
			e := tc.epics[len(tc.epics)-1]
			base, target, err := s.rootRefs("proj", tc.epics, e.Name, e.Tickets[0], repo)
			if err != nil || base != tc.wantBase || target != tc.wantLand {
				t.Errorf("got (%q, %q, %v), want (%q, %q)", base, target, err, tc.wantBase, tc.wantLand)
			}
		})
	}
}

func TestRootRefs_MissingTrunkFailsOnlyARootTargetingTrunk(t *testing.T) {
	s, repo := rootRefsFixture(t)
	runGit(t, repo, "branch", "-D", "develop")
	blocker := tickets.Epic{Name: "a", Tickets: []tickets.Ticket{{Number: 1, Identifier: "01", Status: "done"}}}
	stacked := tickets.Epic{Name: "b", BlockedBy: []string{"a/01"}, Tickets: []tickets.Ticket{{Number: 1, Identifier: "01", Status: "open"}}}
	if base, target, err := s.rootRefs("proj", []tickets.Epic{blocker, stacked}, "b", stacked.Tickets[0], repo); err != nil || base != "a" || target != "a" {
		t.Errorf("stacked root = (%q, %q, %v), want (a, a)", base, target, err)
	}
	var missing *tickets.ErrTrunkMissing
	if _, _, err := s.rootRefs("proj", []tickets.Epic{blocker}, "a", blocker.Tickets[0], repo); !errors.As(err, &missing) {
		t.Errorf("top-level root err = %v, want ErrTrunkMissing", err)
	}
}
