package ghfake

import (
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
func (o *Origin) SimulateSquashMerge(branch string) string {
	o.t.Helper()
	scratch := filepath.Join(o.root, "scratch")
	git(o.t, o.root, nil, "clone", "-q", "--branch", o.Trunk, o.Bare, scratch)
	git(o.t, scratch, nil, "fetch", "-q", o.Local, branch)
	git(o.t, scratch, nil, "merge", "--squash", "FETCH_HEAD")
	git(o.t, scratch, nil, "-c", "user.name=ghfake", "-c", "user.email=ghfake@example.com",
		"commit", "-q", "--allow-empty", "-m", "Squash "+branch)
	sha := strings.TrimSpace(git(o.t, scratch, nil, "rev-parse", "HEAD"))
	git(o.t, scratch, []string{"GHFAKE_QUIET=1"}, "push", "-q", "origin", o.Trunk)
	if err := os.RemoveAll(scratch); err != nil {
		o.t.Fatal(err)
	}
	merged := false
	o.GH.Update(func(s *State) {
		for i := range s.PRs {
			if s.PRs[i].Head == branch {
				s.PRs[i].State, s.PRs[i].MergeCommit, merged = "MERGED", sha, true
			}
		}
	})
	if !merged {
		o.t.Fatalf("SimulateSquashMerge: no fake PR has head %q", branch)
	}
	return sha
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
