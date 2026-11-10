package ghfake_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil/ghfake"
)

func TestMain(m *testing.M) {
	ghfake.Warm()
	os.Exit(m.Run())
}

// gh runs the built program against f's state and returns stdout, stderr and the exit code.
func gh(t *testing.T, f *ghfake.Fake, dir string, args ...string) (string, string, int) {
	t.Helper()
	env := f.Env(nil, "")
	// Resolve gh through the PATH the fake put it on, not this process's.
	path := strings.TrimPrefix(env[len(env)-1], "PATH=")
	cmd := exec.Command(filepath.Join(strings.TrimSuffix(path, string(os.PathListSeparator)), "gh"), args...)
	cmd.Dir = dir
	cmd.Env = env
	var out, errOut strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	code := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(out.String()), strings.TrimSpace(errOut.String()), code
}

func TestScriptedFailure_ExitsWithScriptedCodeAndStderr(t *testing.T) {
	f := ghfake.New(t)
	f.Update(func(s *ghfake.State) {
		s.Failures = []ghfake.Failure{{Match: "auth status", Code: 4, Stderr: "run gh auth login", Times: 1}}
	})
	if _, stderr, code := gh(t, f, "", "auth", "status"); code != 4 || stderr != "run gh auth login" {
		t.Fatalf("first call = %d %q, want 4 and the scripted stderr", code, stderr)
	}
	if _, _, code := gh(t, f, "", "auth", "status"); code != 0 {
		t.Fatalf("second call exit = %d, want 0 once the failure is spent", code)
	}
}

func TestPRCreate_RecordsTitleBodyAndArguments(t *testing.T) {
	o := ghfake.NewOrigin(t, "main")
	body := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(body, []byte("the body"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := gh(t, o.GH, o.Local, "pr", "create", "--draft", "--head", "feat", "--base", "main",
		"--title", "A title", "--body-file", body, "-R", ghfake.Repo)
	if code != 0 || out != "https://github.com/acme/widgets/pull/1" {
		t.Fatalf("pr create = %d %q, want the PR URL", code, out)
	}
	want := ghfake.Created{Repo: ghfake.Repo, Head: "feat", Base: "main", Title: "A title", Body: "the body"}
	if got := o.GH.State().Created; !reflect.DeepEqual(got, []ghfake.Created{want}) {
		t.Fatalf("created = %+v, want %+v", got, want)
	}
	if pr := o.GH.State().PRs[0]; pr.State != "OPEN" || !pr.Draft {
		t.Fatalf("pr = %+v, want an open draft", pr)
	}
}

func TestWatchQuery_AnswersFromState(t *testing.T) {
	f := ghfake.New(t)
	f.Update(func(s *ghfake.State) {
		s.PRs = []ghfake.PR{{Number: 7, State: "OPEN", Draft: true, HeadSHA: "abc", Mergeable: "CONFLICTING"}}
		s.Checks = map[string][]ghfake.Check{"abc": {{Name: "ci", Status: "COMPLETED", Conclusion: "FAILURE"}, {Name: "lint", Status: "IN_PROGRESS"}}}
	})
	out, _, code := gh(t, f, "", "api", "graphql", "-f", "query=query{repository{p7:pullRequest(number:7){state} p8:pullRequest(number:8){state}}}")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	for _, want := range []string{`"headRefOid":"abc","isDraft":true,"mergeCommit":null,"mergeable":"CONFLICTING","state":"OPEN"`, `"statusCheckRollup":{"state":"FAILURE"}`, `"p8":null`} {
		if !strings.Contains(out, want) {
			t.Fatalf("watch answer %s lacks %s", out, want)
		}
	}
}

func TestRerunJob_IsLoggedAndFlipsTheCheck(t *testing.T) {
	f := ghfake.New(t)
	f.Update(func(s *ghfake.State) {
		s.Checks = map[string][]ghfake.Check{"abc": {{Name: "ci", Status: "COMPLETED", Conclusion: "FAILURE", JobID: 11}}}
		s.Reruns = map[string]ghfake.Result{"11": {Status: "IN_PROGRESS"}}
	})
	if _, _, code := gh(t, f, "", "run", "rerun", "--job", "11", "-R", ghfake.Repo); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got := f.State().Checks["abc"][0]; got.Status != "IN_PROGRESS" || got.Conclusion != "" {
		t.Fatalf("check = %+v, want it in progress", got)
	}
	if calls := f.CallsMatching("run rerun --job 11"); len(calls) != 1 {
		t.Fatalf("rerun calls = %v, want one", calls)
	}
}

func TestSquashMerge_LeavesHeadOffTrunkAndLogsPushesAndFetches(t *testing.T) {
	o := ghfake.NewOrigin(t, "main")
	branch := "feat"
	git := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = o.Local
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("checkout", "-q", "-b", branch)
	if err := os.WriteFile(filepath.Join(o.Local, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "work")
	head := git("rev-parse", "HEAD")
	git("push", "-q", "origin", branch)
	o.GH.Update(func(s *ghfake.State) {
		s.PRs = []ghfake.PR{{Number: 1, Head: branch, State: "OPEN", HeadSHA: head}}
	})
	if n := len(o.Pushes()); n != 1 || !strings.HasSuffix(o.Pushes()[0], "refs/heads/"+branch) {
		t.Fatalf("pushes = %v, want just the branch push (the setup and squash pushes are quiet)", o.Pushes())
	}

	sha := o.SimulateSquashMerge(branch)
	git("fetch", "-q", "origin")
	if pr := o.GH.State().PRs[0]; pr.State != "MERGED" || pr.MergeCommit != sha {
		t.Fatalf("pr = %+v, want MERGED at %s", pr, sha)
	}
	if got := git("rev-parse", "origin/main"); got != sha {
		t.Fatalf("origin/main = %s, want the squash %s", got, sha)
	}
	if err := exec.Command("git", "-C", o.Local, "merge-base", "--is-ancestor", head, "origin/main").Run(); err == nil {
		t.Fatal("PR head is an ancestor of trunk, want a squash that is not")
	}
	if len(o.Fetches()) == 0 {
		t.Fatal("fetches log is empty after a fetch")
	}
}
