package ghfake

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
)

// Repo is the owner/repo the origin's URL names, which is what Preflight reads.
const Repo = "acme/widgets"

// Origin is a bare origin, the project repo whose `origin` it is, and the fake
// gh that answers for it.
type Origin struct {
	t     *testing.T
	GH    *Fake
	Bare  string // the bare repo
	Local string // the project repo
	Trunk string
	root  string
}

// NewOrigin makes a bare repo at <root>/acme/widgets.git and a project repo
// whose `origin` points at it, with trunk pushed. Pushes land in a post-receive
// hook's log and fetches in the log of a wrapper around upload-pack, both
// scoped to this origin.
func NewOrigin(t *testing.T, trunk string) *Origin {
	t.Helper()
	root := t.TempDir()
	o := &Origin{t: t, GH: New(t), Bare: filepath.Join(root, filepath.FromSlash(Repo)+".git"), Trunk: trunk, root: root}
	if err := os.MkdirAll(o.Bare, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, o.Bare, nil, "init", "--bare", "-q", "-b", trunk)
	// Pushes and fetches are read back from these two logs; a squash made on
	// the test's behalf is not a push the code under test made.
	hook := "#!/bin/sh\n[ -n \"$GHFAKE_QUIET\" ] || cat >> " + o.log("pushes") + "\n"
	upload := "#!/bin/sh\necho \"$*\" >> " + o.log("fetches") + "\nexec git upload-pack \"$@\"\n"
	for path, body := range map[string]string{filepath.Join(o.Bare, "hooks", "post-receive"): hook, filepath.Join(root, "upload-pack"): upload} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	o.Local = testutil.TempRepo(t)
	if trunk != "main" {
		git(t, o.Local, nil, "branch", "-M", trunk)
	}
	git(t, o.Local, nil, "remote", "add", "origin", o.Bare)
	git(t, o.Local, nil, "config", "remote.origin.uploadpack", filepath.Join(root, "upload-pack"))
	git(t, o.Local, []string{"GHFAKE_QUIET=1"}, "push", "-q", "origin", trunk)
	return o
}

func (o *Origin) log(name string) string { return filepath.Join(o.root, name+".log") }

func (o *Origin) lines(name string) []string {
	data, _ := os.ReadFile(o.log(name))
	if len(data) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

// Pushes is one "old new ref" line per ref a push updated on origin.
func (o *Origin) Pushes() []string { return o.lines("pushes") }

// Fetches is one line per upload-pack run against origin (a fetch or ls-remote).
func (o *Origin) Fetches() []string { return o.lines("fetches") }

// SimulateSquashMerge puts one squash commit of branch (taken from the project
// repo) on origin's trunk, which makes it a commit the branch head is not an
// ancestor of, and marks the fake PR for branch MERGED with that commit. It
// returns the squash commit's SHA.
//
// Its subject is "Squash <branch> (#<number>)", GitHub's mark of a squash;
// SimulateUnmarkedSquashMerge leaves the mark off.
func (o *Origin) SimulateSquashMerge(branch string) string {
	o.t.Helper()
	return o.simulate(branch, func(scratch string, number int) {
		git(o.t, scratch, nil, "merge", "--squash", "FETCH_HEAD")
		o.commit(scratch, "--allow-empty", "-m", fmt.Sprintf("Squash %s (#%d)", branch, number))
	})
}

// SimulateUnmarkedSquashMerge is SimulateSquashMerge with a subject that has no
// "(#<number>)" mark.
func (o *Origin) SimulateUnmarkedSquashMerge(branch string) string {
	o.t.Helper()
	return o.simulate(branch, func(scratch string, _ int) {
		git(o.t, scratch, nil, "merge", "--squash", "FETCH_HEAD")
		o.commit(scratch, "--allow-empty", "-m", "Squash "+branch)
	})
}

// SimulateMergeCommit puts a real merge commit of branch on origin's trunk, as
// GitHub's "Create a merge commit" does, and marks the fake PR MERGED with it.
func (o *Origin) SimulateMergeCommit(branch string) string {
	o.t.Helper()
	return o.simulate(branch, func(scratch string, number int) {
		git(o.t, scratch, nil, "-c", "user.name=ghfake", "-c", "user.email=ghfake@example.com",
			"merge", "-q", "--no-ff", "-m", fmt.Sprintf("Merge pull request #%d from %s", number, branch), "FETCH_HEAD")
	})
}

// SimulateRebaseMerge replays branch's commits on origin's trunk with their own
// headlines, as GitHub's "Rebase and merge" does, and marks the fake PR MERGED
// with the last one. reword changes every headline, the case a headline match
// cannot follow.
func (o *Origin) SimulateRebaseMerge(branch string, reword bool) string {
	o.t.Helper()
	return o.simulate(branch, func(scratch string, _ int) {
		revs := strings.Fields(git(o.t, scratch, nil, "rev-list", "--reverse", "--no-merges", o.Trunk+"..FETCH_HEAD"))
		for _, rev := range revs {
			git(o.t, scratch, nil, "cherry-pick", "--allow-empty", "--keep-redundant-commits", rev)
			if reword {
				subject := strings.TrimSpace(git(o.t, scratch, nil, "log", "-1", "--format=%s"))
				o.commit(scratch, "--amend", "--allow-empty", "-m", "Reworded: "+subject)
			}
		}
	})
}

// simulate clones trunk, fetches branch from the project repo as FETCH_HEAD, runs
// land to put the merge on trunk, pushes it and marks the fake PR for branch
// MERGED with trunk's new tip.
func (o *Origin) simulate(branch string, land func(scratch string, prNumber int)) string {
	o.t.Helper()
	number := 0
	o.GH.Update(func(s *State) {
		for _, p := range s.PRs {
			if p.Head == branch {
				number = p.Number
			}
		}
	})
	if number == 0 {
		o.t.Fatalf("no fake PR has head %q", branch)
	}
	scratch := filepath.Join(o.root, "scratch")
	git(o.t, o.root, nil, "clone", "-q", "--branch", o.Trunk, o.Bare, scratch)
	git(o.t, scratch, nil, "fetch", "-q", o.Local, branch)
	land(scratch, number)
	sha := strings.TrimSpace(git(o.t, scratch, nil, "rev-parse", "HEAD"))
	git(o.t, scratch, []string{"GHFAKE_QUIET=1"}, "push", "-q", "origin", o.Trunk)
	if err := os.RemoveAll(scratch); err != nil {
		o.t.Fatal(err)
	}
	o.GH.Update(func(s *State) {
		for i := range s.PRs {
			if s.PRs[i].Head == branch {
				s.PRs[i].State, s.PRs[i].MergeCommit = "MERGED", sha
			}
		}
	})
	return sha
}

func (o *Origin) commit(dir string, args ...string) {
	o.t.Helper()
	git(o.t, dir, nil, append([]string{"-c", "user.name=ghfake", "-c", "user.email=ghfake@example.com", "commit", "-q"}, args...)...)
}

func git(t testing.TB, dir string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "maintenance.auto=false"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}
