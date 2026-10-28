package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
)

// fakeTools puts shell scripts for names first on a PATH that holds nothing
// else but sh and git, so a tool not named here is not installed. Each script
// appends "<name> <args>" to the returned log. body maps a name to extra shell
// run after logging ($@ are the args).
func fakeTools(t *testing.T, body map[string]string, names ...string) (logPath string) {
	t.Helper()
	bin := t.TempDir()
	logPath = filepath.Join(t.TempDir(), "tools.log")
	for _, name := range names {
		script := "#!/bin/sh\necho \"" + name + " $*\" >> '" + logPath + "'\n" + body[name] + "\n"
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, real := range []string{"sh", "git"} {
		p, err := exec.LookPath(real)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(p, filepath.Join(bin, real)); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	return logPath
}

func readLog(t *testing.T, p string) string {
	t.Helper()
	data, err := os.ReadFile(p)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(data)
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return string(out)
}

func TestCheck_DefaultsRunOnlyRowsOfChangedFiles(t *testing.T) {
	repo := checkRepo(t, `{}`)
	tools := fakeTools(t, nil, "gofmt", "shfmt", "shellcheck", "basedpyright", "ty", "go")
	testutil.WriteFile(t, repo, "a.go", "package a\n")
	testutil.WriteFile(t, repo, "run.sh", "echo hi\n")

	out, code := runCheckCmd(t, repo)
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	got := readLog(t, tools)
	for _, want := range []string{"gofmt -w a.go", "shfmt -w run.sh", "shellcheck run.sh", "go test -json -v ."} {
		if !strings.Contains(got, want) {
			t.Errorf("tool log lacks %q:\n%s", want, got)
		}
	}
	for _, not := range []string{"basedpyright", "ty "} {
		if strings.Contains(got, not) {
			t.Errorf("tool log has %q:\n%s", not, got)
		}
	}
	if !strings.Contains(out, "skip  typecheck  no matching files") {
		t.Errorf("typecheck line:\n%s", out)
	}
}

func TestCheck_DefaultsMatchExtensionlessShebangFiles(t *testing.T) {
	repo := checkRepo(t, `{}`)
	tools := fakeTools(t, nil, "shellcheck", "shfmt")
	testutil.WriteFile(t, repo, "bin-bash", "#!/usr/bin/env bash\necho\n")
	testutil.WriteFile(t, repo, "bin-perl", "#!/usr/bin/perl\n")
	testutil.WriteFile(t, repo, "notes", "just text\n")

	runCheckCmd(t, repo)
	got := readLog(t, tools)
	if !strings.Contains(got, "shellcheck bin-bash\n") || strings.Contains(got, "perl") || strings.Contains(got, "notes") {
		t.Errorf("tool log:\n%s", got)
	}
}

func TestCheck_ConfigWithOnlyTestRunsDefaultsForTheOtherKinds(t *testing.T) {
	repo := checkRepo(t, `{"test":"echo configured-test"}`)
	tools := fakeTools(t, nil, "gofmt", "echo")
	testutil.WriteFile(t, repo, "a.go", "package a\n")

	out, code := runCheckCmd(t, repo)
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if got := readLog(t, tools); !strings.Contains(got, "gofmt -w a.go") {
		t.Errorf("format default did not run:\n%s", got)
	}
	if logOf(t, out, "test") == "" {
		t.Errorf("configured test did not run:\n%s", out)
	}
}

func TestCheck_FullPairWithoutChangedRunsDefaultOnChangedRun(t *testing.T) {
	repo := checkRepo(t, `{"format":{"full":"echo full-format"}}`)
	tools := fakeTools(t, nil, "gofmt")
	testutil.WriteFile(t, repo, "a.go", "package a\n")

	runCheckCmd(t, repo, "--kind", "format")
	if got := readLog(t, tools); !strings.Contains(got, "gofmt -w a.go") {
		t.Errorf("default did not run:\n%s", got)
	}
}

func TestCheck_MissingDefaultToolSkipsButConfiguredMissingToolFails(t *testing.T) {
	repo := checkRepo(t, `{"lint":"no-such-tool-xyz"}`)
	fakeTools(t, nil)
	testutil.WriteFile(t, repo, "run.sh", "echo hi\n")

	out, code := runCheckCmd(t, repo)
	if code != 1 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	for _, want := range []string{"skip  format     shfmt not installed", "fail  lint"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestCheck_NothingRunsWhenEveryDefaultToolIsMissing(t *testing.T) {
	repo := checkRepo(t, `{}`)
	fakeTools(t, nil)
	testutil.WriteFile(t, repo, "run.sh", "echo hi\n")

	if out, code := runCheckCmd(t, repo); code != 3 || !strings.Contains(out, "shellcheck not installed") {
		t.Fatalf("exit %d\n%s", code, out)
	}
}

func TestCheck_ServerFileRunsBriefGoTestNotMakeTarget(t *testing.T) {
	repo := checkRepo(t, `{}`)
	tools := fakeTools(t, nil, "go", "make")
	testutil.WriteFile(t, repo, "Makefile", "test-server:\n\techo sharded\n")
	testutil.WriteFile(t, repo, "server/s.go", "package server\n")
	testutil.CommitAll(t, repo, "server")
	testutil.WriteFile(t, repo, "server/s.go", "package server // changed\n")

	out, code := runCheckCmd(t, repo, "--kind", "test")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	got := readLog(t, tools)
	if !strings.Contains(got, "go test -json -v ./server") || strings.Contains(got, "make") {
		t.Errorf("tool log:\n%s", got)
	}
}

func TestCheck_StagedRestagesFilesFormatRewrote(t *testing.T) {
	repo := checkRepo(t, `{}`)
	fakeTools(t, map[string]string{"gofmt": `echo "// formatted" >> "$2"`}, "gofmt")
	testutil.WriteFile(t, repo, "a.go", "package a\n")
	testutil.MustGitExported(t, repo, "add", "a.go")

	out, code := runCheckCmd(t, repo, "--staged", "--kind", "format")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	staged := gitOut(t, repo, "diff", "--cached", "--", "a.go")
	if !strings.Contains(staged, "// formatted") {
		t.Errorf("rewritten content is not staged:\n%s", staged)
	}
	if left := gitOut(t, repo, "diff", "--", "a.go"); strings.TrimSpace(left) != "" {
		t.Errorf("working tree differs from the index:\n%s", left)
	}
	if !strings.Contains(out, "re-staged 1 files") {
		t.Errorf("output lacks the re-stage note:\n%s", out)
	}
}

func TestCheck_ChangedRunReportsFormatRewriteWithoutStaging(t *testing.T) {
	repo := checkRepo(t, `{}`)
	fakeTools(t, map[string]string{"gofmt": `echo "// formatted" >> "$2"`}, "gofmt")
	testutil.WriteFile(t, repo, "a.go", "package a\n")

	out, _ := runCheckCmd(t, repo, "--kind", "format")
	if !strings.Contains(out, "format changed 1 files in the working tree") {
		t.Errorf("output:\n%s", out)
	}
	if staged := gitOut(t, repo, "diff", "--cached", "--name-only"); strings.TrimSpace(staged) != "" {
		t.Errorf("staged: %q", staged)
	}
}

func TestCheck_FullRunTestsEveryGoDirectory(t *testing.T) {
	repo := checkRepo(t, `{}`)
	tools := fakeTools(t, nil, "go")
	testutil.WriteFile(t, repo, "a.go", "package a\n")
	testutil.WriteFile(t, repo, "pkg/b.go", "package pkg\n")
	testutil.WriteFile(t, repo, "pkg/testdata/c.go", "package c\n")
	testutil.CommitAll(t, repo, "go files")

	out, code := runCheckCmd(t, repo, "--full", "--kind", "test")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if got := readLog(t, tools); !strings.Contains(got, "go test -json -v . ./pkg\n") {
		t.Errorf("tool log:\n%s", got)
	}
}

func TestCheck_FailingGoTestIsNamedUnderTheTestLine(t *testing.T) {
	repo := testutil.TempRepo(t)
	testutil.WriteFile(t, repo, "go.mod", "module example.com/tiny\n\ngo 1.21\n")
	testutil.WriteFile(t, repo, "a_test.go", "package tiny\n\nimport \"testing\"\n\nfunc TestBroken(t *testing.T) { t.Fatal(\"boom\") }\nfunc TestFine(t *testing.T) {}\n")

	out, code := runCheckCmd(t, repo, "--kind", "test")
	if code != 1 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if !strings.Contains(out, "fail  test") || !strings.Contains(out, "\n  example.com/tiny.TestBroken\n") {
		t.Errorf("output lacks the failing test name:\n%s", out)
	}
	if strings.Contains(out, "TestFine") {
		t.Errorf("passing test is named:\n%s", out)
	}
	if !strings.Contains(out, "test log: ") {
		t.Errorf("output lacks the log path:\n%s", out)
	}
}

func TestCheck_BriefCannotRunIsAFail(t *testing.T) {
	repo := checkRepo(t, `{}`)
	// A package with a tsconfig but no node_modules anywhere: tsc cannot be found.
	testutil.WriteFile(t, repo, "web/package.json", `{}`)
	testutil.WriteFile(t, repo, "web/tsconfig.json", `{}`)
	testutil.WriteFile(t, repo, "web/pnpm-lock.yaml", "")
	testutil.WriteFile(t, repo, "web/node_modules/.keep", "")
	testutil.WriteFile(t, repo, "web/a.ts", "export {}\n")

	out, code := runCheckCmd(t, repo, "--kind", "typecheck")
	if code != 1 || !strings.Contains(out, "fail  typecheck") {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if !strings.Contains(out, "tsc not found") {
		t.Errorf("brief's reason is missing:\n%s", out)
	}
}

func TestCheck_JSPackageWithoutNodeModulesInstallsFirst(t *testing.T) {
	repo := checkRepo(t, `{}`)
	tools := fakeTools(t, nil, "pnpm")
	testutil.WriteFile(t, repo, "web/package.json", `{"devDependencies":{"prettier":"3"}}`)
	testutil.WriteFile(t, repo, "web/pnpm-lock.yaml", "")
	testutil.WriteFile(t, repo, "web/a.ts", "export {}\n")

	out, code := runCheckCmd(t, repo, "--kind", "format")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if got := readLog(t, tools); got != "pnpm install --frozen-lockfile\npnpm exec prettier --write a.ts\n" {
		t.Errorf("tool log:\n%s", got)
	}
}

func TestCheck_FailedFrozenInstallFailsTheKindWithItsOutputInTheLog(t *testing.T) {
	repo := checkRepo(t, `{}`)
	fakeTools(t, map[string]string{"pnpm": `echo "lockfile is out of date"; exit 1`}, "pnpm")
	testutil.WriteFile(t, repo, "web/package.json", `{"devDependencies":{"prettier":"3"}}`)
	testutil.WriteFile(t, repo, "web/pnpm-lock.yaml", "")
	testutil.WriteFile(t, repo, "web/a.ts", "export {}\n")

	out, code := runCheckCmd(t, repo, "--kind", "format")
	if code != 1 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if got := logOf(t, out, "format"); !strings.Contains(got, "lockfile is out of date") {
		t.Errorf("log lacks the install output:\n%s", got)
	}
}

func TestCheck_JSPackageWithoutLockfileIsSkipped(t *testing.T) {
	repo := checkRepo(t, `{}`)
	tools := fakeTools(t, nil, "npm", "npx")
	testutil.WriteFile(t, repo, "web/package.json", `{"devDependencies":{"prettier":"3"}}`)
	testutil.WriteFile(t, repo, "web/a.ts", "export {}\n")

	out, code := runCheckCmd(t, repo, "--kind", "format")
	if code != 3 || !strings.Contains(out, "skip  format     web: no lockfile") {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if got := readLog(t, tools); got != "" {
		t.Errorf("a tool ran:\n%s", got)
	}
}

func TestCheck_JSToolListedOnlyInWorkspaceRootRuns(t *testing.T) {
	repo := checkRepo(t, `{}`)
	tools := fakeTools(t, nil, "pnpm")
	testutil.WriteFile(t, repo, "package.json", `{"devDependencies":{"eslint":"9"}}`)
	testutil.WriteFile(t, repo, "pnpm-lock.yaml", "")
	testutil.WriteFile(t, repo, "node_modules/.keep", "")
	testutil.WriteFile(t, repo, "packages/web/package.json", `{}`)
	testutil.WriteFile(t, repo, "packages/web/a.ts", "export {}\n")

	out, code := runCheckCmd(t, repo, "--kind", "lint")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if got := readLog(t, tools); got != "pnpm exec eslint a.ts\n" {
		t.Errorf("tool log (no install, member package runs from its dir):\n%s", got)
	}
}
