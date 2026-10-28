package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/check"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

type finishFixture struct {
	wt, projectDir, ticket string
}

// newFinishFixture registers a bare repo as project p holding epic e (branch
// e) and ticket 01, with an iteration worktree on ralph-loop/e-item-01.
func newFinishFixture(t *testing.T) *finishFixture {
	t.Helper()
	repoDir, _ := mergeTestRepo(t, map[string]string{"e": "e"})
	projectDir := addProject(t, isolateTicketStore(t), "p", repoDir)
	wt := filepath.Join(repoDir, "item-01")
	testutil.MustGitExported(t, repoDir, "worktree", "add", "-b", "ralph-loop/e-item-01", wt, "e")
	testutil.MustGitExported(t, wt, "config", "user.email", "test@test.com")
	testutil.MustGitExported(t, wt, "config", "user.name", "Test")

	testutil.WriteFile(t, filepath.Join(projectDir, "e"), "ticket.md", "---\nstatus: open\n---\n\n# E\n")
	if err := os.MkdirAll(filepath.Join(projectDir, "e", "issues"), 0o755); err != nil {
		t.Fatal(err)
	}
	ticket := filepath.Join(projectDir, "e", "issues", "01-x.md")
	writeTicketFile(t, ticket, "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\nBody.\n")
	writeTicketFile(t, filepath.Join(projectDir, "e", "issues", "02-flaky.md"), "---\nid: \"02\"\nstatus: open\ntype: implement\n---\nBody.\n")
	writeTicketFile(t, filepath.Join(projectDir, "e", "issues", "03-other.md"), "---\nid: \"03\"\nstatus: open\ntype: implement\n---\nBody.\n")
	return &finishFixture{wt: wt, projectDir: projectDir, ticket: ticket}
}

// record appends a check record for the worktree's current tree.
func (f *finishFixture) record(t *testing.T, exit int, kindFilter string, failed ...string) {
	t.Helper()
	tree, err := git.TreeID(f.wt)
	if err != nil {
		t.Fatal(err)
	}
	f.recordTree(t, tree, exit, kindFilter, failed...)
}

func (f *finishFixture) recordTree(t *testing.T, tree string, exit int, kindFilter string, failed ...string) {
	t.Helper()
	rec := check.Record{
		Run: "r", Exit: exit, KindFilter: kindFilter, Branch: "ralph-loop/e-item-01", TreeID: tree,
		Kinds: []check.KindRecord{{Kind: "test", Failed: failed}},
	}
	line, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	dirs := config.DirsFrom(envFor(t))
	dataDir, err := dirs.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dataDir, "check", check.ProjectForDirs(dirs, f.wt)+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.Write(append(line, '\n')); err != nil {
		t.Fatal(err)
	}
}

func (f *finishFixture) finish(t *testing.T, extra ...string) error {
	t.Helper()
	d := deps{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}, getwd: func() (string, error) { return f.wt, nil }, env: envFor(t)}
	return execute(append([]string{"tickets", "set", f.ticket, "--iteration-status=finished"}, extra...), d)
}

func (f *finishFixture) assertUnwritten(t *testing.T) {
	t.Helper()
	raw, err := os.ReadFile(f.ticket)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "iteration_status") {
		t.Errorf("refused call wrote the ticket: %q", raw)
	}
}

func TestExecute_TicketsSet_FinishGuard(t *testing.T) {
	t.Parallel()
	t.Run("commit with no record is refused", func(t *testing.T) {
		f := newFinishFixture(t)
		commitFile(t, f.wt, "a.txt", "a")
		err := f.finish(t)
		if err == nil || !strings.Contains(err.Error(), "gx check") || !strings.Contains(err.Error(), "--check-failures") {
			t.Fatalf("err = %v, want a refusal naming gx check and --check-failures", err)
		}
		f.assertUnwritten(t)
	})

	t.Run("passing run is accepted, also after committing", func(t *testing.T) {
		f := newFinishFixture(t)
		testutil.WriteFile(t, f.wt, "a.txt", "a")
		f.record(t, check.ExitPass, "")
		testutil.MustGitExported(t, f.wt, "add", ".")
		testutil.MustGitExported(t, f.wt, "commit", "-m", "add a")
		if err := f.finish(t); err != nil {
			t.Fatalf("finish: %v", err)
		}
	})

	t.Run("no applicable checks counts as a pass", func(t *testing.T) {
		f := newFinishFixture(t)
		commitFile(t, f.wt, "a.txt", "a")
		f.record(t, check.ExitNone, "")
		if err := f.finish(t); err != nil {
			t.Fatalf("finish: %v", err)
		}
	})

	t.Run("kind-filtered line does not count", func(t *testing.T) {
		f := newFinishFixture(t)
		commitFile(t, f.wt, "a.txt", "a")
		f.record(t, check.ExitPass, "test")
		if err := f.finish(t); err == nil {
			t.Fatal("want a refusal")
		}
		f.assertUnwritten(t)
	})

	t.Run("line for another tree does not count", func(t *testing.T) {
		f := newFinishFixture(t)
		commitFile(t, f.wt, "a.txt", "a")
		f.recordTree(t, "0000000000000000000000000000000000000000", check.ExitPass, "")
		if err := f.finish(t); err == nil {
			t.Fatal("want a refusal")
		}
		f.assertUnwritten(t)
	})

	t.Run("failing line needs one address per failing test", func(t *testing.T) {
		f := newFinishFixture(t)
		commitFile(t, f.wt, "a.txt", "a")
		f.record(t, check.ExitFail, "", "TestA", "TestB")
		if err := f.finish(t); err == nil {
			t.Fatal("no addresses: want a refusal")
		}
		if err := f.finish(t, "--check-failures=02"); err == nil {
			t.Fatal("one address for two tests: want a refusal")
		}
		if err := f.finish(t, "--check-failures=02,99"); err == nil {
			t.Fatal("unknown address: want a refusal")
		}
		f.assertUnwritten(t)
		if err := f.finish(t, "--check-failures=02,03"); err != nil {
			t.Fatalf("two existing addresses: %v", err)
		}
	})

	t.Run("failing line naming no tests needs one address", func(t *testing.T) {
		f := newFinishFixture(t)
		commitFile(t, f.wt, "a.txt", "a")
		f.record(t, check.ExitFail, "")
		if err := f.finish(t); err == nil {
			t.Fatal("want a refusal")
		}
		if err := f.finish(t, "--check-failures=02"); err != nil {
			t.Fatalf("one address: %v", err)
		}
	})

	t.Run("no commits skips the guard", func(t *testing.T) {
		f := newFinishFixture(t)
		if err := f.finish(t); err != nil {
			t.Fatalf("finish: %v", err)
		}
	})
}
