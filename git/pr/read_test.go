package pr_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/git/pr"
	"github.com/elentok/gx/testutil"
)

// fakeGH installs a gh script that prints stdout, prints stderr and exits with
// code, and returns a Runner whose PATH finds it first. Its arguments are
// appended to the returned log file.
func fakeGH(t *testing.T, dir, stdout, stderr string, code int) (pr.Runner, string) {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(bin, "args.log")
	script := "#!/bin/sh\necho \"$@\" >> '" + log + "'\ncat <<'OUT'\n" + stdout +
		"\nOUT\ncat >&2 <<'ERR'\n" + stderr + "\nERR\nexit " + itoa(code) + "\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	env := []string{"PATH=" + bin + ":/usr/bin:/bin:/usr/local/bin:/opt/homebrew/bin", "HOME=" + t.TempDir()}
	return pr.Runner{Env: env, Dir: dir}, log
}

func itoa(n int) string { return string(rune('0' + n)) }

func readLog(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestClassifyViaRunner(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		stderr string
		code   int
		kind   pr.PRListErrorKind
		retry  time.Duration
	}{
		{"exit 4", "To get started with GitHub CLI, please run:  gh auth", 4, pr.PRListErrorUnauthenticated, 0},
		{"403", "gh: Forbidden (HTTP 403)", 1, pr.PRListErrorForbidden, 0},
		{"404", "gh: Not Found (HTTP 404)", 1, pr.PRListErrorNotFound, 0},
		{"429 retry-after", "gh: Too Many Requests (HTTP 429)\nretry-after: 42", 1, pr.PRListErrorRateLimited, 42 * time.Second},
		{"other", "boom", 1, pr.PRListErrorGeneric, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r, _ := fakeGH(t, t.TempDir(), "", tc.stderr, tc.code)
			_, err := r.FindByHead("o/r", "b")
			var le *pr.PRListError
			if !errors.As(err, &le) {
				t.Fatalf("err = %v, want *PRListError", err)
			}
			if le.Kind != tc.kind || le.RetryAfter != tc.retry {
				t.Fatalf("kind=%v retry=%v, want %v %v", le.Kind, le.RetryAfter, tc.kind, tc.retry)
			}
		})
	}
}

func TestRunner_GHNotInstalled(t *testing.T) {
	t.Parallel()
	r := pr.Runner{Env: []string{"PATH=" + t.TempDir()}, Dir: t.TempDir()}
	_, err := r.FindByHead("o/r", "b")
	var le *pr.PRListError
	if !errors.As(err, &le) || le.Kind != pr.PRListErrorGHNotInstalled {
		t.Fatalf("err = %v, want not installed", err)
	}
}

func repoWithOrigin(t *testing.T, origin string) string {
	t.Helper()
	dir := testutil.TempRepo(t)
	if origin != "" {
		testutil.MustGitExported(t, dir, "remote", "add", "origin", origin)
	}
	return dir
}

func TestPreflight(t *testing.T) {
	t.Parallel()
	t.Run("ok", func(t *testing.T) {
		t.Parallel()
		r, _ := fakeGH(t, repoWithOrigin(t, "git@github.com:o/r.git"), "o/r", "", 0)
		repo, err := r.Preflight()
		if err != nil || repo != "o/r" {
			t.Fatalf("repo=%q err=%v", repo, err)
		}
	})
	t.Run("not installed", func(t *testing.T) {
		t.Parallel()
		r := pr.Runner{Env: []string{"PATH=" + t.TempDir()}, Dir: repoWithOrigin(t, "https://github.com/o/r")}
		if _, err := r.Preflight(); err == nil || !strings.Contains(err.Error(), "not installed") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("not logged in", func(t *testing.T) {
		t.Parallel()
		r, _ := fakeGH(t, repoWithOrigin(t, "https://github.com/o/r"), "", "run gh auth login", 4)
		if _, err := r.Preflight(); err == nil || err.Error() != "gh is not logged in, run `gh auth login`" {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("no access", func(t *testing.T) {
		t.Parallel()
		r, _ := fakeGH(t, repoWithOrigin(t, "https://github.com/o/r.git"), "", "Could not resolve to a Repository", 1)
		if _, err := r.Preflight(); err == nil || err.Error() != "gh cannot access o/r" {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("no origin", func(t *testing.T) {
		t.Parallel()
		r, log := fakeGH(t, repoWithOrigin(t, ""), "o/r", "", 0)
		if _, err := r.Preflight(); err == nil || !strings.Contains(err.Error(), "origin") {
			t.Fatalf("err = %v", err)
		}
		if _, statErr := os.Stat(log); statErr == nil {
			t.Fatal("gh ran although there is no origin")
		}
	})
}

func TestFindByHead(t *testing.T) {
	t.Parallel()
	const open = `{"number":3,"url":"u3","state":"OPEN","isDraft":true,"headRefOid":"abc","mergeCommit":null}`
	const merged = `{"number":2,"url":"u2","state":"MERGED","isDraft":false,"headRefOid":"def","mergeCommit":{"oid":"mc"}}`
	const closed = `{"number":1,"url":"u1","state":"CLOSED","isDraft":false,"headRefOid":"ghi","mergeCommit":null}`
	cases := []struct {
		name, out string
		want      int
		state     string
	}{
		{"none", `[]`, 0, ""},
		{"open", `[` + open + `]`, 3, "OPEN"},
		{"open wins", `[` + closed + `,` + merged + `,` + open + `]`, 3, "OPEN"},
		{"merged", `[` + closed + `,` + merged + `]`, 2, "MERGED"},
		{"closed", `[` + closed + `]`, 1, "CLOSED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r, log := fakeGH(t, t.TempDir(), tc.out, "", 0)
			got, err := r.FindByHead("o/r", "feat")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(readLog(t, log), "-R o/r") {
				t.Fatalf("no -R in %q", readLog(t, log))
			}
			if tc.want == 0 {
				if got != nil {
					t.Fatalf("got %+v", got)
				}
				return
			}
			if got == nil || got.Number != tc.want || got.State != tc.state {
				t.Fatalf("got %+v", got)
			}
			if tc.state == "MERGED" && got.MergeCommit != "mc" {
				t.Fatalf("merge commit = %q", got.MergeCommit)
			}
			if tc.state == "OPEN" && (!got.IsDraft || got.HeadSHA != "abc") {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestWatchQuery(t *testing.T) {
	t.Parallel()
	const body = `{"data":{"repository":{
		"p5":{"state":"OPEN","isDraft":true,"headRefOid":"abc","mergeCommit":null,"mergeable":"CONFLICTING","commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"FAILURE"}}}]}},
		"p6":{"state":"MERGED","isDraft":false,"headRefOid":"def","mergeCommit":{"oid":"mc"},"mergeable":"UNKNOWN"}}}}`
	r, log := fakeGH(t, t.TempDir(), body, "", 0)
	got, err := r.WatchQuery("o/r", []int{5, 6})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readLog(t, log), "p5:pullRequest(number:5)") {
		t.Fatalf("aliases missing in %q", readLog(t, log))
	}
	if g := got[5]; g.State != "OPEN" || !g.IsDraft || g.HeadSHA != "abc" || g.Mergeable != "CONFLICTING" || g.Rollup != "FAILURE" {
		t.Fatalf("open: %+v", g)
	}
	if g := got[6]; g.State != "MERGED" || g.MergeCommit != "mc" || g.Mergeable != "" {
		t.Fatalf("merged: %+v", g)
	}
}

func TestPRChecks_FeedsClassifier(t *testing.T) {
	t.Parallel()
	const body = `{"data":{"repository":{"pullRequest":{"commits":{"nodes":[{"commit":{"oid":"abc","statusCheckRollup":{"contexts":{"nodes":[
		{"__typename":"CheckRun","databaseId":11,"name":"build","status":"COMPLETED","conclusion":"FAILURE","detailsUrl":"https://x/runs/22/job/11","checkSuite":{"workflowRun":{"databaseId":22,"workflow":{"databaseId":33}}}},
		{"__typename":"CheckRun","databaseId":12,"name":"lint","status":"IN_PROGRESS","conclusion":null,"detailsUrl":"d","checkSuite":{"workflowRun":null}},
		{"__typename":"StatusContext","context":"ci/legacy","state":"PENDING","targetUrl":"t"}]}}}}]}}}}}`
	r, _ := fakeGH(t, t.TempDir(), body, "", 0)
	sha, checks, err := r.PRChecks("o/r", 9)
	if err != nil || sha != "abc" || len(checks) != 3 {
		t.Fatalf("sha=%q checks=%+v err=%v", sha, checks, err)
	}
	if c := checks[0]; c.JobID != "11" || c.RunID != "22" || c.WorkflowID != "33" || c.URL != "https://x/runs/22/job/11" {
		t.Fatalf("build: %+v", c)
	}
	result, failed := pr.ClassifyChecks(sha, checks)
	if result != pr.ChecksFailed || len(failed) != 1 || failed[0].Name != "build" {
		t.Fatalf("result=%v failed=%v", result, failed)
	}
}

func TestRunnerDoesNotUseProcessEnv(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("gh"); err == nil {
		// A real gh on the process PATH must not leak into a Runner whose env
		// has no gh.
		r := pr.Runner{Env: []string{"PATH=" + t.TempDir()}}
		if _, err := r.Run("--version"); !errors.Is(err, exec.ErrNotFound) {
			t.Fatalf("err = %v, want not found", err)
		}
	}
}
