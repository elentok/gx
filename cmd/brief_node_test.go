package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
)

// nodeStub writes an executable node_modules/.bin/<tool> under dir. It records
// its working directory and arguments to the returned file, prints the sample
// (a file name in brief/testdata, or "" for none) and exits with exit.
func nodeStub(t *testing.T, dir, tool, sample string, exit int) string {
	t.Helper()
	bin := filepath.Join(dir, "node_modules", ".bin")
	testutil.Mkdir(t, bin)
	rec := filepath.Join(t.TempDir(), tool+".args")
	script := "#!/bin/sh\npwd > '" + rec + "'\nfor a in \"$@\"; do echo \"$a\" >> '" + rec + "'; done\n"
	if sample != "" {
		abs, err := filepath.Abs(filepath.Join("..", "brief", "testdata", sample))
		if err != nil {
			t.Fatal(err)
		}
		script += "cat '" + abs + "'\n"
	}
	script += "exit " + string(rune('0'+exit)) + "\n"
	testutil.WriteFile(t, bin, tool, script)
	if err := os.Chmod(filepath.Join(bin, tool), 0o755); err != nil {
		t.Fatal(err)
	}
	return rec
}

func recorded(t *testing.T, rec string) []string {
	t.Helper()
	raw, err := os.ReadFile(rec)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(raw)), "\n")
}

func nodeEnv(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
}

func TestBrief_VitestAndTscPass(t *testing.T) {
	nodeEnv(t)
	repo := testutil.TempRepo(t)
	nodeStub(t, repo, "vitest", "vitest-pass.json", 0)
	nodeStub(t, repo, "tsc", "tsc-pass.txt", 0)
	testutil.WriteFile(t, repo, "tsconfig.json", "{}")

	out, code := runBriefCmd(t, repo, "vitest")
	first := strings.SplitN(out, "\n", 2)[0]
	if code != 0 || !strings.HasPrefix(first, "PASS vitest: 456 tests in ") {
		t.Fatalf("vitest exit %d, out:\n%s", code, out)
	}
	out, code = runBriefCmd(t, repo, "tsc", "--noEmit")
	first = strings.SplitN(out, "\n", 2)[0]
	if code != 0 || !strings.HasPrefix(first, "PASS tsc: no type errors in ") {
		t.Fatalf("tsc exit %d, out:\n%s", code, out)
	}
}

func TestBrief_VitestNoTestsRan(t *testing.T) {
	nodeEnv(t)
	repo := testutil.TempRepo(t)
	nodeStub(t, repo, "vitest", "", 0)
	out, code := runBriefCmd(t, repo, "vitest")
	if code != 0 || !strings.HasPrefix(out, "PASS vitest: no tests ran") {
		t.Fatalf("exit %d, out:\n%s", code, out)
	}
}

func TestBrief_NodeBinaryWalkUp(t *testing.T) {
	nodeEnv(t)
	repo := testutil.TempRepo(t)
	rec := nodeStub(t, repo, "tsc", "tsc-pass.txt", 0)
	web := filepath.Join(repo, "webui")
	testutil.Mkdir(t, filepath.Join(web, "src"))
	testutil.WriteFile(t, web, "tsconfig.json", "{}")

	// A global tsc on PATH must never be used.
	globalBin := t.TempDir()
	globalRec := nodeStub(t, globalBin, "tsc", "", 0)
	t.Setenv("PATH", filepath.Join(globalBin, "node_modules", ".bin")+string(os.PathListSeparator)+os.Getenv("PATH"))

	for _, args := range [][]string{{"-C", web, "tsc", "--noEmit"}, {"-C", filepath.Join(web, "src"), "tsc", "-p", "..", "--noEmit"}} {
		if out, code := runBriefCmd(t, repo, args...); code != 0 {
			t.Fatalf("%v: exit %d, out:\n%s", args, code, out)
		}
	}
	if _, err := os.Stat(globalRec); err == nil {
		t.Error("the global tsc on PATH was used")
	}
	got := recorded(t, rec)
	if evalSymlinks(t, got[0]) != filepath.Join(web, "src") {
		t.Errorf("ran in %q", got[0])
	}
}

func TestBrief_NodeForcedArgs(t *testing.T) {
	nodeEnv(t)
	repo := testutil.TempRepo(t)
	vrec := nodeStub(t, repo, "vitest", "vitest-pass.json", 0)
	trec := nodeStub(t, repo, "tsc", "tsc-pass.txt", 0)
	testutil.WriteFile(t, repo, "tsconfig.json", "{}")

	out, _ := runBriefCmd(t, repo, "-C", repo, "vitest", "--reporter", "verbose", "-t", "foo")
	if !strings.HasPrefix(out, "brief: --reporter replaced with json\nPASS") {
		t.Errorf("out:\n%s", out)
	}
	if got := strings.Join(recorded(t, vrec)[1:], " "); got != "run --reporter=json -t foo" {
		t.Errorf("vitest args = %q", got)
	}

	runBriefCmd(t, repo, "vitest", "related", "src/a.ts", "src/b.ts")
	if got := strings.Join(recorded(t, vrec)[1:], " "); got != "related --run --reporter=json src/a.ts src/b.ts" {
		t.Errorf("vitest related args = %q", got)
	}

	runBriefCmd(t, repo, "-C", repo, "tsc", "--noEmit")
	if got := strings.Join(recorded(t, trec)[1:], " "); got != "--pretty false --noEmit" {
		t.Errorf("tsc args = %q", got)
	}
}

func TestBrief_NodeExit2Causes(t *testing.T) {
	nodeEnv(t)
	repoWith := func(lock string) string {
		repo := testutil.TempRepo(t)
		if lock != "" {
			testutil.WriteFile(t, repo, lock, "")
		}
		return repo
	}
	oneLine := func(t *testing.T, out string, code int, want string) {
		t.Helper()
		if code != 2 || strings.Count(out, "\n") != 1 || !strings.Contains(out, want) {
			t.Errorf("exit %d, out:\n%s\nwant one line with %q", code, out, want)
		}
	}

	for _, flag := range []string{"--watch", "-w"} {
		repo := repoWith("")
		nodeStub(t, repo, "vitest", "", 0)
		out, code := runBriefCmd(t, repo, "vitest", flag)
		oneLine(t, out, code, "--watch")
	}

	for _, tool := range []string{"vitest", "tsc"} {
		out, code := runBriefCmd(t, t.TempDir(), tool)
		oneLine(t, out, code, "no git root")
	}

	for lock, install := range map[string]string{
		"pnpm-lock.yaml":    "pnpm install --frozen-lockfile",
		"package-lock.json": "npm ci",
		"bun.lock":          "bun install --frozen-lockfile",
		"bun.lockb":         "bun install --frozen-lockfile",
		"yarn.lock":         "yarn install --frozen-lockfile",
		"":                  "no lockfile was found",
	} {
		repo := repoWith(lock)
		out, code := runBriefCmd(t, repo, "vitest")
		oneLine(t, out, code, install)
		if _, err := os.Stat(filepath.Join(repo, "node_modules")); err == nil {
			t.Errorf("%s: brief created node_modules", lock)
		}
	}

	repo := repoWith("")
	nodeStub(t, repo, "tsc", "", 0)
	out, code := runBriefCmd(t, repo, "tsc")
	oneLine(t, out, code, "no tsconfig")
}

// vitestFailRepo returns a repo whose stub vitest prints the recorded report
// of 9 failing tests (7 in "seven", one nested, one with a long message).
func vitestFailRepo(t *testing.T) string {
	t.Helper()
	nodeEnv(t)
	repo := testutil.TempRepo(t)
	nodeStub(t, repo, "vitest", "vitest-fail.json", 1)
	return repo
}

func rerunOf(out string) string {
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "re-run only the failing tests: ") {
			return l
		}
	}
	return ""
}

func TestBrief_VitestFailureLayoutAndLimits(t *testing.T) {
	out, code := runBriefCmd(t, vitestFailRepo(t), "vitest")
	if code != 1 || !strings.HasPrefix(out, "FAIL vitest: 9 failed in ") {
		t.Fatalf("exit %d, out:\n%s", code, out)
	}

	// Seam 1: location, then name, then message; stack frames are dropped.
	want := "/repo/webui/src/tmpbrief7.test.ts\n  long message\n    Error: line 0\n"
	if !strings.Contains(out, want) || !strings.Contains(out, "  seven fails 1\n    AssertionError: expected 1 to be +0 // Object.is equality\n") {
		t.Errorf("layout differs from the Go report:\n%s", out)
	}
	if strings.Contains(out, "    at ") {
		t.Errorf("stack frames shown:\n%s", out)
	}
	if !strings.Contains(out, "  outer inner nested fails\n") {
		t.Errorf("nested describe name missing:\n%s", out)
	}
	if !strings.Contains(out, "line 9") || strings.Contains(out, "line 10") || !strings.Contains(out, "more lines cut") {
		t.Errorf("long message not limited to 10 lines:\n%s", out)
	}

	// Seam 2: 5 in full, the rest by name.
	if !strings.Contains(out, "  seven fails 4\n    AssertionError") || strings.Contains(out, "  seven fails 5\n    AssertionError") {
		t.Errorf("not exactly 5 units in full:\n%s", out)
	}
	if !strings.Contains(out, "  seven fails 7\n") || !strings.Contains(out, "4 more failures are listed by name only") {
		t.Errorf("by-name units missing:\n%s", out)
	}
}

func TestBrief_VitestRerunKeepsArgsAndC(t *testing.T) {
	repo := vitestFailRepo(t)
	out, code := runBriefCmd(t, t.TempDir(), "-C", repo, "vitest", "-t", "seven", "src/a.test.ts")
	if code != 1 {
		t.Fatalf("exit %d, out:\n%s", code, out)
	}
	rerun := rerunOf(out)
	for _, want := range []string{"gx brief -C " + repo + " vitest src/a.test.ts -t '^(", "seven fails 1", "seven fails 7", "outer inner nested fails", "long message", ")$'"} {
		if !strings.Contains(rerun, want) {
			t.Errorf("re-run line lacks %q: %q", want, rerun)
		}
	}
	if strings.Contains(rerun, "-t seven ") {
		t.Errorf("caller's -t kept: %q", rerun)
	}
}

func TestBrief_VitestCrashedReporterFallsBackToRawHead(t *testing.T) {
	nodeEnv(t)
	repo := testutil.TempRepo(t)
	nodeStub(t, repo, "vitest", "vitest-truncated.json", 1)
	out, code := runBriefCmd(t, repo, "vitest")
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if code != 1 || !strings.HasPrefix(out, "FAIL vitest: exit 1 in ") || len(lines) != 1+1+1 || rerunOf(out) != "" {
		t.Errorf("exit %d, out:\n%s", code, out)
	}
}

func TestBrief_VitestLogHasFullOutputAndIsLast(t *testing.T) {
	out, _ := runBriefCmd(t, vitestFailRepo(t), "vitest")
	logPath := lastLine(out)
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(logPath, "-vitest.log") || !strings.Contains(string(raw), "line 29") || !strings.Contains(string(raw), strings.Repeat("x", 1000)) {
		t.Errorf("log %s lacks the cut text", logPath)
	}
}

func TestBrief_NodeUnparsedFailureFallsBackToRawHead(t *testing.T) {
	nodeEnv(t)
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "tsconfig.json", "{}")
	for _, tool := range []string{"vitest", "tsc"} {
		nodeStub(t, repo, tool, "", 1)
	}
	for _, tool := range []string{"vitest", "tsc"} {
		out, code := runBriefCmd(t, repo, tool)
		if code != 1 || !strings.HasPrefix(out, "FAIL "+tool+": exit 1 in ") || !strings.Contains(lastLine(out), "-"+tool+".log") {
			t.Errorf("%s: exit %d, out:\n%s", tool, code, out)
		}
	}
}
