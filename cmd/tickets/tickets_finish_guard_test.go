package tickets

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/check"
	"github.com/elentok/gx/cmd/cmdtest"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/git/history"
	"github.com/elentok/gx/testutil"
)

type finishFixture struct {
	wt, projectDir, ticket string
	head                   string // Head stamped on records; empty for none
}

// newFinishFixture registers a bare repo as project p holding epic e (branch
// e) and ticket 01, with an iteration worktree on ralph-loop/e-item-01.
func newFinishFixture(t *testing.T) *finishFixture {
	t.Helper()
	repoDir, _ := cmdtest.MergeTestRepo(t, map[string]string{"e": "e"})
	projectDir := cmdtest.AddProject(t, cmdtest.IsolateTicketStore(t), "p", repoDir)
	wt := filepath.Join(repoDir, "item-01")
	testutil.MustGitExported(t, repoDir, "worktree", "add", "-b", "ralph-loop/e-item-01", wt, "e")
	testutil.MustGitExported(t, wt, "config", "user.email", "test@test.com")
	testutil.MustGitExported(t, wt, "config", "user.name", "Test")

	testutil.WriteFile(t, filepath.Join(projectDir, "e"), "ticket.md", "---\nstatus: open\n---\n\n# E\n")
	if err := os.MkdirAll(filepath.Join(projectDir, "e", "issues"), 0o755); err != nil {
		t.Fatal(err)
	}
	ticket := filepath.Join(projectDir, "e", "issues", "01-x.md")
	cmdtest.WriteTicketFile(t, ticket, "---\nid: \"01\"\nstatus: claimed\ntype: implement\n---\nBody.\n")
	cmdtest.WriteTicketFile(t, filepath.Join(projectDir, "e", "issues", "02-flaky.md"), "---\nid: \"02\"\nstatus: open\ntype: implement\n---\nBody.\n")
	cmdtest.WriteTicketFile(t, filepath.Join(projectDir, "e", "issues", "03-other.md"), "---\nid: \"03\"\nstatus: open\ntype: implement\n---\nBody.\n")
	return &finishFixture{wt: wt, projectDir: projectDir, ticket: ticket}
}

// record appends a check record for the worktree's current tree.
func (f *finishFixture) record(t *testing.T, exit int, kindFilter string, failed ...string) {
	t.Helper()
	tree, err := history.TreeID(f.wt)
	if err != nil {
		t.Fatal(err)
	}
	f.recordTree(t, tree, exit, kindFilter, failed...)
}

func (f *finishFixture) recordTree(t *testing.T, tree string, exit int, kindFilter string, failed ...string) {
	t.Helper()
	rec := check.Record{
		Run: "r", Head: f.head, Exit: exit, KindFilter: kindFilter, Branch: "ralph-loop/e-item-01", TreeID: tree,
		Kinds: []check.KindRecord{{Kind: "test", Failed: failed}},
	}
	line, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	dirs := config.DirsFrom(cmdtest.EnvFor(t))
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
	d := testDeps(t, &bytes.Buffer{}, &bytes.Buffer{}, func() (string, error) { return f.wt, nil })
	return execute(t, append([]string{"tickets", "set", f.ticket, "--iteration-status=finished"}, extra...), d)
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
		cmdtest.CommitFile(t, f.wt, "a.txt", "a")
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
		cmdtest.CommitFile(t, f.wt, "a.txt", "a")
		f.record(t, check.ExitNone, "")
		if err := f.finish(t); err != nil {
			t.Fatalf("finish: %v", err)
		}
	})

	t.Run("a skipped run after a failed one does not clear the failure", func(t *testing.T) {
		f := newFinishFixture(t)
		cmdtest.CommitFile(t, f.wt, "a.txt", "a")
		f.record(t, check.ExitFail, "", "TestA")
		f.record(t, check.ExitNone, "")
		if err := f.finish(t); err == nil {
			t.Fatal("want a refusal: the skip must not replace the failed run")
		}
		f.assertUnwritten(t)
		if err := f.finish(t, "--check-failures=02"); err != nil {
			t.Fatalf("follow-up named: %v", err)
		}
	})

	t.Run("kind-filtered line does not count", func(t *testing.T) {
		f := newFinishFixture(t)
		cmdtest.CommitFile(t, f.wt, "a.txt", "a")
		f.record(t, check.ExitPass, "test")
		if err := f.finish(t); err == nil {
			t.Fatal("want a refusal")
		}
		f.assertUnwritten(t)
	})

	t.Run("line for another tree does not count", func(t *testing.T) {
		f := newFinishFixture(t)
		cmdtest.CommitFile(t, f.wt, "a.txt", "a")
		f.recordTree(t, "0000000000000000000000000000000000000000", check.ExitPass, "")
		if err := f.finish(t); err == nil {
			t.Fatal("want a refusal")
		}
		f.assertUnwritten(t)
	})

	t.Run("failing line needs one address per failing test", func(t *testing.T) {
		f := newFinishFixture(t)
		cmdtest.CommitFile(t, f.wt, "a.txt", "a")
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

	t.Run("refusal names the open follow-up that mentions the failed test", func(t *testing.T) {
		f := newFinishFixture(t)
		cmdtest.CommitFile(t, f.wt, "a.txt", "a")
		issues := filepath.Join(f.projectDir, "follow-ups", "issues")
		if err := os.MkdirAll(issues, 0o755); err != nil {
			t.Fatal(err)
		}
		testutil.WriteFile(t, filepath.Join(f.projectDir, "follow-ups"), "ticket.md", "---\nstatus: open\n---\n\n# Follow-ups\n")
		cmdtest.WriteTicketFile(t, filepath.Join(issues, "07-hang.md"), "---\nid: \"07\"\nstatus: open\ntype: research\n---\n\n# check-lock hangs\n\n## Problem\nTestScenarios_Start/check-lock hangs.\n")
		cmdtest.WriteTicketFile(t, filepath.Join(issues, "08-done.md"), "---\nid: \"08\"\nstatus: done\ntype: research\n---\n\n# TestScenarios_Start is fixed\n")
		f.record(t, check.ExitFail, "", "TestScenarios_Start/check-lock")
		err := f.finish(t)
		if err == nil || !strings.Contains(err.Error(), "known flake: p:follow-ups/07") || strings.Contains(err.Error(), "follow-ups/08") {
			t.Fatalf("want a refusal naming only p:follow-ups/07, got %v", err)
		}
		f.record(t, check.ExitFail, "", "TestUnrelated")
		if err := f.finish(t); err == nil || strings.Contains(err.Error(), "known flake") {
			t.Fatalf("unmatched test: want a refusal without a known-flake line, got %v", err)
		}
	})

	t.Run("refusal for several failures names --known-failures", func(t *testing.T) {
		f := newFinishFixture(t)
		cmdtest.CommitFile(t, f.wt, "a.txt", "a")
		f.record(t, check.ExitFail, "", "TestA", "TestB")
		err := f.finish(t, "--check-failures=02")
		if err == nil || !strings.Contains(err.Error(), "--known-failures") {
			t.Fatalf("want a refusal naming --known-failures, got %v", err)
		}
		f.record(t, check.ExitFail, "", "TestA")
		if err := f.finish(t); err == nil || strings.Contains(err.Error(), "--known-failures") {
			t.Fatalf("one failure: want a refusal without the hint, got %v", err)
		}
	})

	t.Run("failures already red on the base are waived", func(t *testing.T) {
		f := newFinishFixture(t)
		base, err := git.MergeBase(f.wt, "HEAD", "e")
		if err != nil {
			t.Fatal(err)
		}
		f.head = base
		f.recordTree(t, "base-tree", check.ExitFail, "", "TestA")
		f.head = ""
		cmdtest.CommitFile(t, f.wt, "a.txt", "a")
		f.record(t, check.ExitFail, "", "TestA")
		if err := f.finish(t); err != nil {
			t.Fatalf("only the base's failure: %v", err)
		}
		f.record(t, check.ExitFail, "", "TestA", "TestB")
		if err := f.finish(t); err == nil || !strings.Contains(err.Error(), "at least 1 follow-up") {
			t.Fatalf("one new failure: want a refusal needing 1 address, got %v", err)
		}
		if err := f.finish(t, "--check-failures=02"); err != nil {
			t.Fatalf("one address for the new failure: %v", err)
		}
	})

	t.Run("known failures: one address covers the failing set", func(t *testing.T) {
		f := newFinishFixture(t)
		cmdtest.CommitFile(t, f.wt, "a.txt", "a")
		f.record(t, check.ExitFail, "", "TestA", "TestB")
		if err := f.finish(t, "--known-failures=99"); err == nil {
			t.Fatal("unknown address: want a refusal")
		}
		f.assertUnwritten(t)
		if err := f.finish(t, "--known-failures=02"); err != nil {
			t.Fatalf("one known address for two tests: %v", err)
		}
	})

	t.Run("failing line naming no tests needs one address", func(t *testing.T) {
		f := newFinishFixture(t)
		cmdtest.CommitFile(t, f.wt, "a.txt", "a")
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
