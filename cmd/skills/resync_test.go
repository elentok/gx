package skills_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	skillscmd "github.com/elentok/gx/cmd/skills"
	"github.com/elentok/gx/skills"
)

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func TestSkillsInstallBare_ResyncsSourcesInTheirOwnMode(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	claude, _ := agentRootsOf(t, d)
	linked, copied := writeSource(t), filepath.Join(t.TempDir(), "copied")
	writeFile(t, filepath.Join(copied, "delta", "SKILL.md"), "delta")
	if err := run(t, d, "install", linked); err != nil {
		t.Fatal(err)
	}
	if err := run(t, d, "install", "--copy", copied); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(linked, "gamma", "SKILL.md"), "gamma")
	if err := os.RemoveAll(filepath.Join(linked, "beta")); err != nil {
		t.Fatal(err)
	}

	if err := run(t, d, "install"); err != nil {
		t.Fatalf("bare install: %v", err)
	}

	if !exists(filepath.Join(claude, "gamma")) {
		t.Error("added skill gamma has no link")
	}
	info, err := os.Lstat(filepath.Join(claude, "gamma"))
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("gamma should stay a link: %v %v", info, err)
	}
	if exists(filepath.Join(claude, "beta")) {
		t.Error("removed skill beta still linked")
	}
}

func TestSkillsInstallBare_CopySourceKeepsCopyMode(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	claude, _ := agentRootsOf(t, d)
	src := writeSource(t)
	if err := run(t, d, "install", "--copy", src); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(src, "gamma", "SKILL.md"), "gamma")

	if err := run(t, d, "install"); err != nil {
		t.Fatalf("bare install: %v", err)
	}

	info, err := os.Lstat(filepath.Join(claude, "gamma", "SKILL.md"))
	if err != nil || !info.Mode().IsRegular() {
		t.Errorf("gamma/SKILL.md should be a copied file: %v %v", info, err)
	}
}

func TestSkillsInstallBare_GoneSourceKeepsLinksAndFails(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	claude, _ := agentRootsOf(t, d)
	src := writeSource(t)
	if err := run(t, d, "install", src); err != nil {
		t.Fatal(err)
	}
	moved := src + "-moved"
	if err := os.Rename(src, moved); err != nil {
		t.Fatal(err)
	}

	err := run(t, d, "install")

	if err == nil || !strings.Contains(err.Error(), "gx skills uninstall ") {
		t.Fatalf("err = %v, want one naming `gx skills uninstall <dir>`", err)
	}
	if !exists(filepath.Join(claude, "alpha")) {
		t.Error("links were removed for a gone source")
	}
	if !exists(filepath.Join(claude, "gx-verify", "SKILL.md")) {
		t.Error("bundle was not installed")
	}
	if p := skills.ReadProblem(statusPathOf(t, d)); !strings.Contains(p, "gx skills uninstall ") {
		t.Errorf("problem = %q, want the gone-source problem", p)
	}
}

func TestSkillsInstallBare_BrokenSourceDoesNotStopOthers(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	claude, _ := agentRootsOf(t, d)
	broken := filepath.Join(t.TempDir(), "broken")
	writeFile(t, filepath.Join(broken, "bad", "SKILL.md"), "bad")
	good := filepath.Join(t.TempDir(), "good")
	writeFile(t, filepath.Join(good, "fine", "SKILL.md"), "fine")
	for _, dir := range []string{broken, good} {
		if err := run(t, d, "install", dir); err != nil {
			t.Fatal(err)
		}
	}
	// The person repoints bad's link: a conflict on the next sync.
	link := filepath.Join(claude, "bad")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(good, "extra", "SKILL.md"), "extra")

	err := run(t, d, "install")

	if err == nil || !strings.Contains(err.Error(), "broken") || strings.Contains(err.Error(), "good") {
		t.Fatalf("err = %v, want only the broken source's conflict", err)
	}
	if !exists(filepath.Join(claude, "extra")) || !exists(filepath.Join(claude, "gx-verify", "SKILL.md")) {
		t.Error("the good source or the bundle was stopped by the broken one")
	}
	if p := skills.ReadProblem(statusPathOf(t, d)); !strings.Contains(p, "broken") {
		t.Errorf("problem = %q, want the conflict recorded", p)
	}
}

func TestSkillsInstallDir_LeavesProblemRecordAlone(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	if err := skills.WriteProblem(statusPathOf(t, d), "earlier problem"); err != nil {
		t.Fatal(err)
	}

	if err := run(t, d, "install", writeSource(t)); err != nil {
		t.Fatal(err)
	}

	if p := skills.ReadProblem(statusPathOf(t, d)); p != "earlier problem" {
		t.Errorf("problem = %q, want it untouched", p)
	}
}

func TestRefreshSkills_SourcesJoinProblemKeepGoingAndClear(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	d := fakeSkillsDeps(t, &out)
	claude, _ := agentRootsOf(t, d)
	gone := filepath.Join(t.TempDir(), "gone")
	writeFile(t, filepath.Join(gone, "old", "SKILL.md"), "old")
	good := filepath.Join(t.TempDir(), "good")
	writeFile(t, filepath.Join(good, "fine", "SKILL.md"), "fine")
	for _, dir := range []string{gone, good} {
		if err := run(t, d, "install", dir); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(good, "extra", "SKILL.md"), "extra")
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}

	skillscmd.Refresh(d, &out)

	if !exists(filepath.Join(claude, "extra")) {
		t.Error("good source was not synced past the gone one")
	}
	if p := skills.ReadProblem(statusPathOf(t, d)); !strings.Contains(p, "gx skills uninstall ") {
		t.Errorf("problem = %q, want the gone-source problem", p)
	}

	writeFile(t, filepath.Join(gone, "old", "SKILL.md"), "old")
	skillscmd.Refresh(d, &out)
	if p := skills.ReadProblem(statusPathOf(t, d)); p != "" {
		t.Errorf("problem after the source came back = %q, want none", p)
	}
}
