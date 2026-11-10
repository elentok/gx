package skills_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
)

func listOut(t *testing.T, d *bytes.Buffer, run1 func() error) string {
	t.Helper()
	d.Reset()
	if err := run1(); err != nil {
		t.Fatalf("list: %v", err)
	}
	return d.String()
}

func TestSkillsList_States(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	list := func() string { return listOut(t, &stdout, func() error { return run(t, d, "list") }) }

	if got := list(); !strings.HasPrefix(got, "gx skill bundle: not installed\n") || !strings.Contains(got, "no skill sources") {
		t.Errorf("before install = %q", got)
	}

	if err := run(t, d, "install"); err != nil {
		t.Fatal(err)
	}
	claude, codex := agentRootsOf(t, d)
	got := list()
	if !strings.HasPrefix(got, "gx skill bundle: managed-copy, roots "+claude+", "+codex+"\n") || !strings.Contains(got, "no skill sources") {
		t.Errorf("bundle only = %q", got)
	}

	src1, src2 := writeSource(t), filepath.Join(t.TempDir(), "other")
	writeFile(t, filepath.Join(src2, "gamma", "SKILL.md"), "gamma")
	for _, src := range []string{src1, src2} {
		if err := run(t, d, "install", src); err != nil {
			t.Fatal(err)
		}
	}
	got = list()
	real1, _ := filepath.EvalSymlinks(src1)
	real2, _ := filepath.EvalSymlinks(src2)
	if !strings.Contains(got, "source "+real1+": symlink, roots "+claude+", "+codex+", 2 skills\n") ||
		!strings.Contains(got, "source "+real2+": symlink, roots "+claude+", "+codex+", 1 skills\n") ||
		strings.Contains(got, "no skill sources") {
		t.Errorf("two sources = %q", got)
	}
}

func TestSkillsList_DevInstallListedFirst(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	repo := testutil.TempRepo(t)
	d := fakeSkillsDevDeps(t, &stdout, repo)
	writeDevBundle(t, repo)
	if err := run(t, d, "install", "--dev"); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := run(t, d, "list"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stdout.String(), "gx dev checkout ") || !strings.Contains(stdout.String(), ": symlink, roots ") {
		t.Errorf("dev list = %q", stdout.String())
	}
}

func TestSkillsInstallSource_WarnsAboutCodexHomeDuplicate(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	home := t.TempDir()
	dup := filepath.Join(home, "skills", "alpha")
	if err := os.MkdirAll(dup, 0o755); err != nil {
		t.Fatal(err)
	}
	d.CodexHome = func() string { return home }

	if err := run(t, d, "install", writeSource(t)); err != nil {
		t.Fatalf("install must exit 0: %v", err)
	}
	if !strings.Contains(stdout.String(), "warning: skills: "+dup) {
		t.Errorf("output = %q, want a duplicate warning", stdout.String())
	}
	if _, err := os.Stat(dup); err != nil {
		t.Errorf("duplicate must not be deleted: %v", err)
	}
}
