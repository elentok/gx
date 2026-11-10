package skills_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/skills"
)

func sourceMode(t *testing.T, manifestDir string) skills.InstallMode {
	t.Helper()
	paths, _ := filepath.Glob(filepath.Join(manifestDir, "*.json"))
	if len(paths) != 1 {
		t.Fatalf("want one source manifest, got %v", paths)
	}
	m, err := skills.Load(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	return m.Mode
}

func isLink(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&os.ModeSymlink != 0
}

func TestSkillsInstallSource_ModeSwitchNeedsNoForceWhenIntact(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	claude, codex := agentRootsOf(t, d)
	manifestDir, _ := d.ManifestDir()
	src := writeSource(t)

	if err := run(t, d, "install", src); err != nil {
		t.Fatal(err)
	}
	if err := run(t, d, "install", "--copy", src); err != nil {
		t.Fatalf("link to copy: %v", err)
	}
	for _, root := range []string{claude, codex} {
		if isLink(filepath.Join(root, "alpha")) || !exists(filepath.Join(root, "alpha", "SKILL.md")) {
			t.Errorf("%s/alpha should be a real copy", root)
		}
	}
	if got := sourceMode(t, manifestDir); got != skills.ModeManagedCopy {
		t.Errorf("mode = %q, want copy", got)
	}

	if err := run(t, d, "install", src); err != nil {
		t.Fatalf("copy to link: %v", err)
	}
	for _, root := range []string{claude, codex} {
		if !isLink(filepath.Join(root, "alpha")) {
			t.Errorf("%s/alpha should be a link again", root)
		}
	}
	if got := sourceMode(t, manifestDir); got != skills.ModeSymlink {
		t.Errorf("mode = %q, want symlink", got)
	}
}

func TestSkillsInstallSource_ModeSwitchBlockedByModifiedCopyUntilForced(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	claude, _ := agentRootsOf(t, d)
	manifestDir, _ := d.ManifestDir()
	src := writeSource(t)
	if err := run(t, d, "install", "--copy", src); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(claude, "alpha", "SKILL.md"), "edited by hand")

	if err := run(t, d, "install", src); err == nil {
		t.Fatal("switch with a modified copy should fail")
	}
	if isLink(filepath.Join(claude, "beta")) || !exists(filepath.Join(claude, "beta", "SKILL.md")) {
		t.Error("beta switched although the source's switch was blocked")
	}
	if got := sourceMode(t, manifestDir); got != skills.ModeManagedCopy {
		t.Errorf("mode = %q, record must keep copy", got)
	}

	if err := run(t, d, "install", "--force", "alpha", src); err != nil {
		t.Fatalf("forced switch: %v", err)
	}
	if !isLink(filepath.Join(claude, "alpha")) || !isLink(filepath.Join(claude, "beta")) {
		t.Error("forced switch should link every skill")
	}
	if got := sourceMode(t, manifestDir); got != skills.ModeSymlink {
		t.Errorf("mode = %q, want symlink", got)
	}
}

func TestSkillsInstallSource_ModeSwitchBlockedByRetargetedLink(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	claude, _ := agentRootsOf(t, d)
	manifestDir, _ := d.ManifestDir()
	src := writeSource(t)
	if err := run(t, d, "install", src); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	link := filepath.Join(claude, "alpha")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, link); err != nil {
		t.Fatal(err)
	}

	if err := run(t, d, "install", "--copy", src); err == nil {
		t.Fatal("switch with a repointed link should fail")
	}
	if got := sourceMode(t, manifestDir); got != skills.ModeSymlink {
		t.Errorf("mode = %q, record must keep symlink", got)
	}
}

func TestSkillsInstallBare_NeverSwitchesMode(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	claude, _ := agentRootsOf(t, d)
	manifestDir, _ := d.ManifestDir()
	src := writeSource(t)
	if err := run(t, d, "install", src); err != nil {
		t.Fatal(err)
	}
	if err := run(t, d, "install"); err != nil {
		t.Fatal(err)
	}
	if !isLink(filepath.Join(claude, "alpha")) || sourceMode(t, manifestDir) != skills.ModeSymlink {
		t.Error("bare install must keep the recorded mode")
	}
}
