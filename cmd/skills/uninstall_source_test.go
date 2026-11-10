package skills_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillsUninstallSource_LeavesOtherSourceAndBundle(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	claude, codex := agentRootsOf(t, d)
	one, two := writeSource(t), writeSource(t)
	writeFile(t, filepath.Join(two, "gamma", "SKILL.md"), "gamma")
	if err := os.RemoveAll(filepath.Join(two, "alpha")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(two, "beta")); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"install"}, {"install", one}, {"install", two}} {
		if err := run(t, d, args...); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	thirdParty := filepath.Join(claude, "third-party")
	if err := os.Symlink(one, thirdParty); err != nil {
		t.Fatal(err)
	}
	manifestDir, _ := d.ManifestDir()

	stdout.Reset()
	if err := run(t, d, "uninstall", one); err != nil {
		t.Fatalf("uninstall source: %v", err)
	}

	for _, root := range []string{claude, codex} {
		for _, name := range []string{"alpha", "beta"} {
			if exists(filepath.Join(root, name)) {
				t.Errorf("%s/%s still present", root, name)
			}
		}
		if !exists(filepath.Join(root, "gamma")) {
			t.Errorf("%s/gamma, from the other source, was removed", root)
		}
	}
	if !exists(filepath.Join(claude, "gx.md")) {
		t.Error("bundle was removed")
	}
	if !exists(thirdParty) {
		t.Error("third-party link was removed")
	}
	if got := manifestCount(t, manifestDir); got != 1 {
		t.Errorf("source manifests = %d, want 1", got)
	}
	if !exists(filepath.Join(one, "alpha", "SKILL.md")) {
		t.Error("source folder was touched")
	}
}

func TestSkillsUninstallSource_KeepsModifiedEntryUntilForced(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	claude, _ := agentRootsOf(t, d)
	src := writeSource(t)
	if err := run(t, d, "install", "--copy", src); err != nil {
		t.Fatal(err)
	}
	edited := filepath.Join(claude, "alpha", "SKILL.md")
	writeFile(t, edited, "locally edited")
	manifestDir, _ := d.ManifestDir()

	stdout.Reset()
	if err := run(t, d, "uninstall", src); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if got, _ := os.ReadFile(edited); string(got) != "locally edited" {
		t.Errorf("modified entry = %q, want kept", got)
	}
	if !strings.Contains(stdout.String(), "conflicted") {
		t.Errorf("output lacks conflicted: %q", stdout.String())
	}
	if exists(filepath.Join(claude, "beta", "SKILL.md")) {
		t.Error("unmodified entry was kept")
	}
	if got := manifestCount(t, manifestDir); got != 1 {
		t.Errorf("record dropped while an entry is kept: %d manifests", got)
	}

	if err := run(t, d, "uninstall", src, "--force", "alpha/SKILL.md"); err != nil {
		t.Fatalf("forced uninstall: %v", err)
	}
	if exists(edited) {
		t.Error("forced entry still present")
	}
	if got := manifestCount(t, manifestDir); got != 0 {
		t.Errorf("manifests = %d after forced uninstall, want 0", got)
	}
}

func TestSkillsUninstallSource_SourceFolderDeleted(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	claude, _ := agentRootsOf(t, d)
	src := writeSource(t)
	if err := run(t, d, "install", src); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(src); err != nil {
		t.Fatal(err)
	}

	if err := run(t, d, "uninstall", src); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if exists(filepath.Join(claude, "alpha")) {
		t.Error("dangling link left behind")
	}
	manifestDir, _ := d.ManifestDir()
	if got := manifestCount(t, manifestDir); got != 0 {
		t.Errorf("manifests = %d, want 0", got)
	}
}

func TestSkillsUninstallSource_UnknownSourceReportsNotInstalled(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	if err := run(t, d, "uninstall", filepath.Join(t.TempDir(), "nope")); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if !strings.Contains(stdout.String(), "not installed") {
		t.Errorf("output = %q", stdout.String())
	}
}

func TestSkillsUninstall_BareLeavesSources(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	claude, _ := agentRootsOf(t, d)
	src := writeSource(t)
	for _, args := range [][]string{{"install"}, {"install", src}} {
		if err := run(t, d, args...); err != nil {
			t.Fatal(err)
		}
	}
	if err := run(t, d, "uninstall"); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(claude, "gx.md")) {
		t.Error("bundle kept")
	}
	if !exists(filepath.Join(claude, "alpha")) {
		t.Error("bare uninstall removed a source")
	}
}
