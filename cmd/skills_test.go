package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/skills"
)

// fakeSkillsDeps builds a deps whose skills roots and manifest path are fake
// user locations under t.TempDir(), so tests never touch a real home
// directory - the "fake user locations" test seam ticket 05 calls for.
func fakeSkillsDeps(t *testing.T, stdout *bytes.Buffer) deps {
	t.Helper()
	claudeRoot := filepath.Join(t.TempDir(), "claude-skills")
	codexRoot := filepath.Join(t.TempDir(), "codex-prompts")
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")

	return deps{
		stdout: stdout,
		stderr: bytes.NewBuffer(nil),
		skillsAgentRoots: func() ([]string, error) {
			return []string{claudeRoot, codexRoot}, nil
		},
		skillsManifestPath: func() (string, error) {
			return manifestPath, nil
		},
	}
}

func agentRootsOf(t *testing.T, d deps) (claude, codex string) {
	t.Helper()
	roots, err := d.skillsAgentRoots()
	if err != nil {
		t.Fatalf("skillsAgentRoots: %v", err)
	}
	return roots[0], roots[1]
}

func TestExecute_SkillsInstall_CopiesBundleIntoBothAgentRoots(t *testing.T) {
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	claudeRoot, codexRoot := agentRootsOf(t, d)

	if err := execute([]string{"skills", "install"}, d); err != nil {
		t.Fatalf("execute skills install: %v", err)
	}

	for _, root := range []string{claudeRoot, codexRoot} {
		for _, rel := range []string{"README.md", "local-tracker.md", "gx-to-tickets/SKILL.md", "gx-tdd/SKILL.md", "gx-tdd/tests.md", "gx-tdd/mocking.md"} {
			if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
				t.Errorf("expected %s under %s: %v", rel, root, err)
			}
		}
	}

	out := stdout.String()
	if !strings.Contains(out, "installed") {
		t.Errorf("expected output to report installed targets, got: %q", out)
	}
	if !strings.Contains(out, "README.md") {
		t.Errorf("expected output to name installed files, got: %q", out)
	}
}

func TestExecute_SkillsInstall_ReinstallReportsSkipped(t *testing.T) {
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)

	if err := execute([]string{"skills", "install"}, d); err != nil {
		t.Fatalf("first install: %v", err)
	}

	stdout.Reset()
	if err := execute([]string{"skills", "install"}, d); err != nil {
		t.Fatalf("second install: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "skipped") {
		t.Errorf("expected reinstall to report skipped targets, got: %q", out)
	}
	if strings.Contains(out, "installed") {
		t.Errorf("expected reinstall to report no newly-installed targets, got: %q", out)
	}
}

func TestExecute_SkillsInstall_ConflictedPathBlocksInstallAndReportsConflict(t *testing.T) {
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	claudeRoot, _ := agentRootsOf(t, d)

	if err := os.MkdirAll(claudeRoot, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(claudeRoot, "README.md"), []byte("not gx's"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	err := execute([]string{"skills", "install"}, d)
	if err == nil {
		t.Fatal("expected error from unrelated collision")
	}
	if !strings.Contains(stdout.String(), "conflicted") {
		t.Errorf("expected output to report conflicted target, got: %q", stdout.String())
	}

	got, readErr := os.ReadFile(filepath.Join(claudeRoot, "README.md"))
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}
	if string(got) != "not gx's" {
		t.Errorf("conflicted file was overwritten: %q", got)
	}
}

func TestExecute_SkillsInstall_ForceOverridesConflict(t *testing.T) {
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	claudeRoot, _ := agentRootsOf(t, d)

	if err := os.MkdirAll(claudeRoot, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(claudeRoot, "README.md"), []byte("not gx's"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := execute([]string{"skills", "install", "--force", "README.md"}, d); err != nil {
		t.Fatalf("execute skills install --force: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(claudeRoot, "README.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) == "not gx's" {
		t.Errorf("expected --force to overwrite the conflicted file")
	}
}

func TestExecute_SkillsUninstall_NotInstalledPrintsMessageAndSucceeds(t *testing.T) {
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)

	if err := execute([]string{"skills", "uninstall"}, d); err != nil {
		t.Fatalf("execute skills uninstall: %v", err)
	}
	if !strings.Contains(stdout.String(), "not installed") {
		t.Errorf("expected a not-installed message, got: %q", stdout.String())
	}
}

func TestExecute_SkillsUninstall_RemovesInstalledBundle(t *testing.T) {
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	claudeRoot, codexRoot := agentRootsOf(t, d)

	if err := execute([]string{"skills", "install"}, d); err != nil {
		t.Fatalf("install: %v", err)
	}

	stdout.Reset()
	if err := execute([]string{"skills", "uninstall"}, d); err != nil {
		t.Fatalf("execute skills uninstall: %v", err)
	}

	for _, root := range []string{claudeRoot, codexRoot} {
		if _, err := os.Stat(filepath.Join(root, "README.md")); !os.IsNotExist(err) {
			t.Errorf("expected README.md removed from %s: err = %v", root, err)
		}
	}

	out := stdout.String()
	if !strings.Contains(out, "removed") {
		t.Errorf("expected output to report removed targets, got: %q", out)
	}

	manifestPath, _ := d.skillsManifestPath()
	if _, err := skills.Load(manifestPath); err == nil {
		t.Error("expected manifest to be removed after full uninstall")
	}
}

func TestExecute_SkillsUninstall_PreservesLocallyModifiedFileAsConflicted(t *testing.T) {
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	claudeRoot, _ := agentRootsOf(t, d)

	if err := execute([]string{"skills", "install"}, d); err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := os.WriteFile(filepath.Join(claudeRoot, "README.md"), []byte("locally edited"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	stdout.Reset()
	if err := execute([]string{"skills", "uninstall"}, d); err != nil {
		t.Fatalf("execute skills uninstall: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(claudeRoot, "README.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "locally edited" {
		t.Errorf("expected locally modified README.md preserved, got: %q", got)
	}
	if !strings.Contains(stdout.String(), "conflicted") {
		t.Errorf("expected output to report the preserved file as conflicted, got: %q", stdout.String())
	}
}
