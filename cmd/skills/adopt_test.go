package skills_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillsInstallSource_FirstInstallAdoptsMatchingLink(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	claude, codex := agentRootsOf(t, d)
	src := writeSource(t)
	for _, root := range []string{claude, codex} {
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(src, "alpha"), filepath.Join(root, "alpha")); err != nil {
			t.Fatal(err)
		}
	}

	if err := run(t, d, "install", src); err != nil {
		t.Fatalf("install: %v", err)
	}
	for _, root := range []string{claude, codex} {
		for _, name := range []string{"alpha", "beta"} {
			if !isLink(filepath.Join(root, name)) {
				t.Errorf("%s/%s should be a link", root, name)
			}
		}
	}
	manifestDir, _ := d.ManifestDir()
	if n := manifestCount(t, manifestDir); n != 1 {
		t.Errorf("manifest count = %d, want 1", n)
	}
}

func TestSkillsInstallSource_FirstInstallRemovesDanglingLinkIntoSource(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	claude, _ := agentRootsOf(t, d)
	src := writeSource(t)
	if err := os.MkdirAll(claude, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(src, "gone"), filepath.Join(claude, "alpha")); err != nil {
		t.Fatal(err)
	}

	if err := run(t, d, "install", src); err != nil {
		t.Fatalf("install: %v", err)
	}
	got, err := os.Readlink(filepath.Join(claude, "alpha"))
	if err != nil || got != filepath.Join(src, "alpha") {
		t.Errorf("alpha link = %q, %v; want %q", got, err, filepath.Join(src, "alpha"))
	}
}

func TestSkillsInstallSource_FirstInstallConflictsAbortWholeSource(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	claude, codex := agentRootsOf(t, d)
	src := writeSource(t)
	other := t.TempDir()
	writeFile(t, filepath.Join(claude, "alpha", "SKILL.md"), "mine")
	writeFile(t, filepath.Join(claude, "synced", "keep.md"), "keep")
	if err := os.MkdirAll(codex, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, filepath.Join(codex, "beta")); err != nil {
		t.Fatal(err)
	}

	err := run(t, d, "install", src)
	if err == nil {
		t.Fatal("install should conflict")
	}
	for _, want := range []string{filepath.Join(claude, "alpha"), filepath.Join(codex, "beta")} {
		if !strings.Contains(err.Error()+stdout.String(), want) {
			t.Errorf("output should name %s: %v / %s", want, err, stdout.String())
		}
	}
	if isLink(filepath.Join(claude, "alpha")) || !exists(filepath.Join(claude, "alpha", "SKILL.md")) {
		t.Error("claude/alpha must be untouched")
	}
	if got, _ := os.Readlink(filepath.Join(codex, "beta")); got != other {
		t.Errorf("codex/beta link = %q, want untouched", got)
	}
	if exists(filepath.Join(claude, "beta")) || exists(filepath.Join(codex, "alpha")) {
		t.Error("no entry of the source may be written")
	}
	manifestDir, _ := d.ManifestDir()
	if n := manifestCount(t, manifestDir); n != 0 {
		t.Errorf("manifest count = %d, want 0", n)
	}

	if err := run(t, d, "install", "--force", "alpha", src); err == nil {
		t.Fatal("forcing only alpha should still conflict on beta")
	}
	if !exists(filepath.Join(claude, "alpha", "SKILL.md")) || isLink(filepath.Join(claude, "alpha")) {
		t.Error("partial force must not write anything")
	}

	if err := run(t, d, "install", "--force-all", src); err != nil {
		t.Fatalf("force-all: %v", err)
	}
	if !isLink(filepath.Join(claude, "alpha")) || !isLink(filepath.Join(codex, "beta")) {
		t.Error("force-all should replace both")
	}
	if !exists(filepath.Join(claude, "synced", "keep.md")) {
		t.Error("synced/ must stay untouched")
	}
	if err := run(t, d, "install", src); err != nil {
		t.Fatalf("rerun: %v", err)
	}
	if !exists(filepath.Join(claude, "synced", "keep.md")) {
		t.Error("synced/ must survive rerun")
	}
}

func TestSkillsInstallSource_ForcePathReplacesRealFolders(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	claude, codex := agentRootsOf(t, d)
	src := writeSource(t)
	writeFile(t, filepath.Join(claude, "alpha", "SKILL.md"), "mine")
	writeFile(t, filepath.Join(codex, "alpha", "SKILL.md"), "mine too")

	if err := run(t, d, "install", "--force", "alpha", src); err != nil {
		t.Fatalf("force alpha: %v", err)
	}
	for _, root := range []string{claude, codex} {
		if !isLink(filepath.Join(root, "alpha")) {
			t.Errorf("%s/alpha should be replaced", root)
		}
	}
}
