package server

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop"
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
		epics               []tickets.Epic
		mergeBlocker        bool
		wantBase, wantMerge string
	}{
		"no base starts from and lands on trunk": {
			epics: []tickets.Epic{{Name: "b", Tickets: []tickets.Ticket{{Number: 1, Identifier: "01", Status: "open"}}}}, wantBase: "develop", wantMerge: "develop",
		},
		"commitless ref is trunk": {
			epics: []tickets.Epic{{Name: "b", Tickets: []tickets.Ticket{{Number: 1, Identifier: "01", Status: "open", Commitless: true}}}}, wantBase: "develop", wantMerge: "develop",
		},
		"explicit base moves only the start":             {epics: stacked("release-1.2"), wantBase: "release-1.2", wantMerge: "develop"},
		"stacked root targets its unlanded blocker":      {epics: stacked(""), wantBase: "a", wantMerge: "a"},
		"stacked root targets trunk once blocker landed": {epics: stacked(""), mergeBlocker: true, wantBase: "develop", wantMerge: "develop"},
	} {
		t.Run(name, func(t *testing.T) {
			s, repo := rootRefsFixture(t)
			if tc.mergeBlocker {
				runGit(t, repo, "branch", "-f", "develop", "a")
			}
			e := tc.epics[len(tc.epics)-1]
			base, target, err := s.rootRefs("proj", tc.epics, e.Name, e.Tickets[0], repo)
			if err != nil || base != tc.wantBase || target != tc.wantMerge {
				t.Errorf("got (%q, %q, %v), want (%q, %q)", base, target, err, tc.wantBase, tc.wantMerge)
			}
		})
	}
}

func TestRootRefs_MergedBlockerWithDeletedBranchTargetsTrunk(t *testing.T) {
	s, repo := rootRefsFixture(t)
	runGit(t, repo, "branch", "-m", "a", "c")
	blocker := tickets.Epic{Name: "a", MergedAt: time.Now(), Tickets: []tickets.Ticket{{Number: 1, Identifier: "01", Status: "done"}}}
	other := tickets.Epic{Name: "c", Tickets: []tickets.Ticket{{Number: 1, Identifier: "01", Status: "done"}}}
	for name, tc := range map[string]struct {
		dependent           tickets.Ticket
		epics               []tickets.Epic
		wantBase, wantMerge string
	}{
		"derived from a merged blocker":   {tickets.Ticket{Number: 1, Identifier: "01", Status: "open", BlockedBy: []string{"a/01"}}, []tickets.Epic{blocker}, "develop", "develop"},
		"explicit base on merged blocker": {tickets.Ticket{Number: 1, Identifier: "01", Status: "open", Base: "a/01"}, []tickets.Epic{blocker}, "develop", "develop"},
		"merged blocker is not ambiguous": {tickets.Ticket{Number: 1, Identifier: "01", Status: "open", BlockedBy: []string{"a/01", "c/01"}}, []tickets.Epic{blocker, other}, "c", "c"},
	} {
		t.Run(name, func(t *testing.T) {
			dependent := tickets.Epic{Name: "b", Tickets: []tickets.Ticket{tc.dependent}}
			base, target, err := s.rootRefs("proj", append(tc.epics, dependent), "b", tc.dependent, repo)
			if err != nil || base != tc.wantBase || target != tc.wantMerge {
				t.Errorf("got (%q, %q, %v), want (%q, %q)", base, target, err, tc.wantBase, tc.wantMerge)
			}
		})
	}
}

// rootLowestEpics is epic b whose file order and finish order both differ
// from its numbers: the cancelled 01, stacked on a, is the Root ticket; 02
// and 03 carry a base: of their own that targets trunk.
func rootLowestEpics() []tickets.Epic {
	return []tickets.Epic{
		{Name: "a", Tickets: []tickets.Ticket{{Number: 1, Identifier: "01", Status: "done"}}},
		{Name: "b", Tickets: []tickets.Ticket{
			{Number: 3, Identifier: "03", Status: "done", Base: "release-1.2"},
			{Number: 1, Identifier: "01", Status: "cancelled", BlockedBy: []string{"a/01"}},
			{Number: 2, Identifier: "02", Status: "open", Base: "release-1.2"},
		}},
	}
}

func TestEpicRefs_EveryCallerUsesTheLowestTicket(t *testing.T) {
	s, repo := rootRefsFixture(t)
	epics := rootLowestEpics()
	finished, claimed := epics[1].Tickets[0], epics[1].Tickets[2]

	root, base, target, err := s.epicRefs("proj", epics, "b", repo)
	if err != nil || root.Identifier != "01" || base != "a" || target != "a" {
		t.Fatalf("epicRefs = (%s, %q, %q, %v), want (01, a, a)", root.Identifier, base, target, err)
	}
	ref, stamp, _, err := s.resolveBase(tickets.Address{Project: "proj", Epic: "b", ID: "02"}, claimed, repo, epics)
	if want := "a@" + revParse(t, repo, "a"); err != nil || ref != "a" || stamp != want {
		t.Errorf("resolveBase = (%q, %q, %v), want (a, %s)", ref, stamp, err, want)
	}
	// b holds only a's work: landed on the root's target a, not on trunk.
	runGit(t, repo, "branch", "b", "a")
	if !s.nothingToMerge("proj", epics, ralphloop.OneIteration{Epic: "b", Ticket: finished}, repo) {
		t.Errorf("nothingToMerge = false, want b already on its target a")
	}
}

func TestResolveBase_ExistingFeatureBranchKeepsItsBase(t *testing.T) {
	s, repo := rootRefsFixture(t)
	runGit(t, repo, "branch", "b", "main")
	epics := rootLowestEpics()
	_, stamp, _, err := s.resolveBase(tickets.Address{Project: "proj", Epic: "b", ID: "02"}, epics[1].Tickets[2], repo, epics)
	if want := "a@" + revParse(t, repo, "main"); err != nil || stamp != want {
		t.Errorf("stamp = %q, %v, want %s", stamp, err, want)
	}
}

func revParse(t *testing.T, repo, ref string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", repo, "rev-parse", ref).Output()
	if err != nil {
		t.Fatalf("rev-parse %s: %v", ref, err)
	}
	return strings.TrimSpace(string(out))
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
