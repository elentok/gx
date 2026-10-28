package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
)

// runCheckCmd drives `gx check` through the root command and returns stdout
// and the exit code.
func runCheckCmd(t *testing.T, cwd string, args ...string) (string, int) {
	t.Helper()
	root := newRootCmd(deps{getwd: func() (string, error) { return cwd, nil }, env: envFor(t)})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"check"}, args...))
	err := root.Execute()
	var ee *ExitError
	if errors.As(err, &ee) {
		return out.String(), ee.Code
	}
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	return out.String(), 0
}

// checkRepo is a TempRepo with checksJSON committed as .gx/checks.json and a
// fresh data dir.
func checkRepo(t *testing.T, checksJSON string) string {
	t.Helper()
	repo := testutil.TempRepo(t)
	testutil.Mkdir(t, filepath.Join(repo, ".gx"))
	testutil.WriteFile(t, repo, ".gx/checks.json", checksJSON)
	testutil.CommitAll(t, repo, "checks")
	return repo
}

// logOf reads the log of kind from the run whose summary path is out's first line.
func logOf(t *testing.T, out, kind string) string {
	t.Helper()
	summary := strings.SplitN(out, "\n", 2)[0]
	data, err := os.ReadFile(filepath.Join(filepath.Dir(summary), kind+".log"))
	if err != nil {
		t.Fatalf("log of %s: %v\n%s", kind, err, out)
	}
	return string(data)
}

func TestCheck_ConfiguredTestKindAlsoFromLinkedWorktree(t *testing.T) {
	repo, wt := testutil.TempRepoWithLinkedWorktree(t, "feature")
	for _, dir := range []string{repo, wt} {
		testutil.Mkdir(t, filepath.Join(dir, ".gx"))
		testutil.WriteFile(t, dir, ".gx/checks.json", `{"test":"echo ran in $(basename $PWD)"}`)
		out, code := runCheckCmd(t, dir, "--kind", "test")
		if code != 0 || !strings.Contains(out, "pass  test") {
			t.Fatalf("%s: exit %d\n%s", dir, code, out)
		}
		if got := logOf(t, out, "test"); !strings.Contains(got, "ran in "+filepath.Base(dir)) {
			t.Errorf("%s: log = %q", dir, got)
		}
	}
}

func TestCheck_OffReadsSkip(t *testing.T) {
	repo := checkRepo(t, `{"lint":false,"test":"true"}`)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	out, code := runCheckCmd(t, repo)
	if code != 0 || !strings.Contains(out, "skip  lint       off") {
		t.Fatalf("exit %d\n%s", code, out)
	}
}

func TestCheck_FilesPlaceholderSelection(t *testing.T) {
	repo := checkRepo(t, `{"format":"echo {files}"}`)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.WriteFile(t, repo, "b.txt", "b\n")
	testutil.CommitAll(t, repo, "ab")
	testutil.WriteFile(t, repo, "a.txt", "a2\n")
	testutil.WriteFile(t, repo, "c.txt", "c\n")

	out, code := runCheckCmd(t, repo, "--kind", "format")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if got := logOf(t, out, "format"); got != "a.txt c.txt\n" {
		t.Errorf("changed run log = %q", got)
	}

	out, _ = runCheckCmd(t, repo, "--kind", "format", "--full")
	got := logOf(t, out, "format")
	for _, f := range []string{".gx/checks.json", "a.txt", "b.txt"} {
		if !strings.Contains(got, f) {
			t.Errorf("full run log lacks %s: %q", f, got)
		}
	}
	if strings.Contains(got, "c.txt") {
		t.Errorf("full run lists the untracked c.txt: %q", got)
	}
}

func TestCheck_ChangedPairMissingSkipsUntilFull(t *testing.T) {
	repo := checkRepo(t, `{"test":{"full":"echo full"}}`)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	out, code := runCheckCmd(t, repo, "--kind", "test")
	if code != 3 || !strings.Contains(out, "skip  test") {
		t.Fatalf("changed run: exit %d\n%s", code, out)
	}
	out, code = runCheckCmd(t, repo, "--kind", "test", "--full")
	if code != 0 || logOf(t, out, "test") != "full\n" {
		t.Fatalf("full run: exit %d\n%s", code, out)
	}
}

func TestCheck_ProjectJSONFallbackAndRepoFileWins(t *testing.T) {
	repo := testutil.TempRepo(t)
	projectDir := registerWithTrunk(t, repo, "main")
	testutil.WriteFile(t, projectDir, "project.json",
		`{"name":"p","repo":"`+repo+`","trunk":"main","checks":{"test":"echo from-project"}}`)

	out, code := runCheckCmd(t, repo, "--full", "--kind", "test")
	if code != 0 || logOf(t, out, "test") != "from-project\n" {
		t.Fatalf("fallback: exit %d\n%s", code, out)
	}

	testutil.Mkdir(t, filepath.Join(repo, ".gx"))
	testutil.WriteFile(t, repo, ".gx/checks.json", `{"test":"echo from-repo"}`)
	out, code = runCheckCmd(t, repo, "--full", "--kind", "test")
	if code != 0 || logOf(t, out, "test") != "from-repo\n" {
		t.Fatalf("repo file: exit %d\n%s", code, out)
	}
}

func TestCheck_SinceAndKind(t *testing.T) {
	repo := checkRepo(t, `{"format":"echo {files}","lint":"echo lint"}`)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	testutil.CommitAll(t, repo, "a")
	testutil.WriteFile(t, repo, "b.txt", "b\n")
	testutil.CommitAll(t, repo, "b")

	out, code := runCheckCmd(t, repo, "--since", "HEAD~1", "--kind", "format")
	if code != 0 || logOf(t, out, "format") != "b.txt\n" {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if strings.Contains(out, "lint") {
		t.Errorf("--kind format printed a lint line:\n%s", out)
	}
}

func TestCheck_FailureShowsSummaryLogAndLast20Lines(t *testing.T) {
	repo := checkRepo(t, `{"test":"seq 1 30; exit 1","lint":"no-such-tool-xyz"}`)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	out, code := runCheckCmd(t, repo)
	if code != 1 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	summary := strings.SplitN(out, "\n", 2)[0]
	if !strings.HasSuffix(summary, "summary.txt") {
		t.Errorf("first line = %q", summary)
	}
	for _, want := range []string{"fail  lint", "fail  test", "test log: " + filepath.Join(filepath.Dir(summary), "test.log"), "\n  11\n", "\n  30\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\n  10\n") {
		t.Errorf("tail has more than 20 lines:\n%s", out)
	}
}

func TestCheck_ExitCodes(t *testing.T) {
	repo := checkRepo(t, `{"test":"true"}`)
	if out, code := runCheckCmd(t, repo); code != 3 || !strings.Contains(out, "no applicable checks") {
		t.Errorf("clean tree: exit %d\n%s", code, out)
	}
	if out, code := runCheckCmd(t, repo, "--full"); code != 0 {
		t.Errorf("full: exit %d\n%s", code, out)
	}
	if out, code := runCheckCmd(t, repo, "--kind", "bogus"); code != 2 {
		t.Errorf("unknown kind: exit %d\n%s", code, out)
	}
	if out, code := runCheckCmd(t, repo, "--staged", "--full"); code != 2 {
		t.Errorf("exclusive flags: exit %d\n%s", code, out)
	}
	if out, code := runCheckCmd(t, repo, "--since", "no-such-ref"); code != 2 {
		t.Errorf("bad ref: exit %d\n%s", code, out)
	}
	if out, code := runCheckCmd(t, t.TempDir(), "--full"); code != 2 {
		t.Errorf("not a repo: exit %d\n%s", code, out)
	}
	testutil.WriteFile(t, repo, ".gx/checks.json", `{not json`)
	out, code := runCheckCmd(t, repo, "--full")
	if code != 2 || strings.Count(strings.TrimRight(out, "\n"), "\n") != 0 {
		t.Errorf("invalid JSON: exit %d, want one line\n%s", code, out)
	}
	testutil.WriteFile(t, repo, ".gx/checks.json", `{"build":"true"}`)
	if out, code = runCheckCmd(t, repo, "--full"); code != 2 || !strings.Contains(out, "build") {
		t.Errorf("unknown config kind: exit %d\n%s", code, out)
	}
}

func TestCheck_FormatFinishesBeforeLintAndOutputOrderIsFixed(t *testing.T) {
	stamps := t.TempDir()
	repo := checkRepo(t, `{
		"test": "true",
		"typecheck": "true",
		"lint": "test -f `+stamps+`/format.done",
		"format": "sleep 0.3 && touch `+stamps+`/format.done"
	}`)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	out, code := runCheckCmd(t, repo)
	if code != 0 {
		t.Fatalf("lint ran before format finished: exit %d\n%s", code, out)
	}
	last := -1
	for _, kind := range []string{"format", "lint", "typecheck", "test"} {
		i := strings.Index(out, "pass  "+kind)
		if i <= last {
			t.Errorf("%s out of order:\n%s", kind, out)
		}
		last = i
	}
}

func TestCheck_SummaryFileSurvivesDiscardedOutput(t *testing.T) {
	repo := checkRepo(t, `{"test":"true"}`)
	testutil.WriteFile(t, repo, "a.txt", "a\n")
	out, _ := runCheckCmd(t, repo)
	summary := strings.SplitN(out, "\n", 2)[0]
	data, err := os.ReadFile(summary)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != out {
		t.Errorf("summary file differs from output:\n%s\n--\n%s", data, out)
	}
}
