package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/skills"
	"github.com/elentok/gx/testutil"
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

// fakeSkillsDevDeps is fakeSkillsDeps with getwd faked to cwd, the seam
// runSkillsInstallDev uses to resolve the invoking git checkout.
func fakeSkillsDevDeps(t *testing.T, stdout *bytes.Buffer, cwd string) deps {
	t.Helper()
	d := fakeSkillsDeps(t, stdout)
	d.getwd = func() (string, error) { return cwd, nil }
	return d
}

// writeDevBundle writes a fake copy of gx's skill bundle - the same
// relative layout Bundle embeds - under root/skills/, so dev-mode install
// tests can point resolveDevRoot at an isolated temp checkout instead of
// depending on this repo's real skills/ directory. Returns root's skills/
// directory.
func writeDevBundle(t *testing.T, root string) string {
	t.Helper()
	rels, err := skills.BundleFiles()
	if err != nil {
		t.Fatalf("BundleFiles: %v", err)
	}
	bundleDir := filepath.Join(root, "skills")
	for _, rel := range rels {
		abs := filepath.Join(bundleDir, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(abs, []byte("dev content: "+rel), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	return bundleDir
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

func TestExecute_SkillsInstallDev_SymlinksCurrentCheckoutIntoBothAgentRoots(t *testing.T) {
	var stdout bytes.Buffer
	repo := testutil.TempRepo(t)
	bundleDir := writeDevBundle(t, repo)
	d := fakeSkillsDevDeps(t, &stdout, repo)
	claudeRoot, codexRoot := agentRootsOf(t, d)

	if err := execute([]string{"skills", "install", "--dev"}, d); err != nil {
		t.Fatalf("execute skills install --dev: %v", err)
	}

	for _, root := range []string{claudeRoot, codexRoot} {
		linkPath := filepath.Join(root, "gx-implement", "SKILL.md")
		target, err := os.Readlink(linkPath)
		if err != nil {
			t.Fatalf("Readlink(%s): %v", linkPath, err)
		}
		want := filepath.Join(bundleDir, "gx-implement", "SKILL.md")
		if target != want {
			t.Errorf("symlink target = %q, want %q", target, want)
		}
	}

	out := stdout.String()
	if !strings.Contains(out, "installed") {
		t.Errorf("expected output to report installed targets, got: %q", out)
	}
}

func TestExecute_SkillsInstallDev_ResolvesGitRootFromNestedWorkingDirectory(t *testing.T) {
	var stdout bytes.Buffer
	repo := testutil.TempRepo(t)
	bundleDir := writeDevBundle(t, repo)
	nested := filepath.Join(repo, "cmd", "sub")
	testutil.Mkdir(t, nested)
	d := fakeSkillsDevDeps(t, &stdout, nested)
	claudeRoot, _ := agentRootsOf(t, d)

	if err := execute([]string{"skills", "install", "--dev"}, d); err != nil {
		t.Fatalf("execute skills install --dev: %v", err)
	}

	linkPath := filepath.Join(claudeRoot, "README.md")
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink(%s): %v", linkPath, err)
	}
	if target != filepath.Join(bundleDir, "README.md") {
		t.Errorf("symlink target = %q, want repo root's bundle (%q)", target, bundleDir)
	}
}

func TestExecute_SkillsInstallDev_ResolvesLinkedWorktreeRootNotBareRoot(t *testing.T) {
	var stdout bytes.Buffer
	bareRepo := testutil.TempBareRepoWithWorktrees(t, "feature")
	wtDir := filepath.Join(bareRepo, "feature")
	bundleDir := writeDevBundle(t, wtDir)
	d := fakeSkillsDevDeps(t, &stdout, wtDir)
	claudeRoot, _ := agentRootsOf(t, d)

	if err := execute([]string{"skills", "install", "--dev"}, d); err != nil {
		t.Fatalf("execute skills install --dev: %v", err)
	}

	linkPath := filepath.Join(claudeRoot, "README.md")
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink(%s): %v", linkPath, err)
	}
	if target != filepath.Join(bundleDir, "README.md") {
		t.Errorf("symlink target = %q, want the worktree's bundle (%q), not the bare repo's", target, bundleDir)
	}
}

func TestExecute_SkillsInstallDev_ReinstallFromSameCheckoutIsIdempotent(t *testing.T) {
	var stdout bytes.Buffer
	repo := testutil.TempRepo(t)
	writeDevBundle(t, repo)
	d := fakeSkillsDevDeps(t, &stdout, repo)

	if err := execute([]string{"skills", "install", "--dev"}, d); err != nil {
		t.Fatalf("first install --dev: %v", err)
	}

	stdout.Reset()
	if err := execute([]string{"skills", "install", "--dev"}, d); err != nil {
		t.Fatalf("second install --dev: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "skipped") {
		t.Errorf("expected reinstall to report skipped targets, got: %q", out)
	}
	if strings.Contains(out, "conflicted") {
		t.Errorf("expected idempotent reinstall to report no conflicts, got: %q", out)
	}
}

func TestExecute_SkillsInstallDev_DifferentCheckoutDetectsChangedLinkTargetInsteadOfRetargeting(t *testing.T) {
	var stdout bytes.Buffer
	repoA := testutil.TempRepo(t)
	bundleDirA := writeDevBundle(t, repoA)
	dA := fakeSkillsDevDeps(t, &stdout, repoA)
	claudeRoot, _ := agentRootsOf(t, dA)

	if err := execute([]string{"skills", "install", "--dev"}, dA); err != nil {
		t.Fatalf("install from repoA: %v", err)
	}

	repoB := testutil.TempRepo(t)
	writeDevBundle(t, repoB)
	dB := dA
	dB.getwd = func() (string, error) { return repoB, nil }

	stdout.Reset()
	err := execute([]string{"skills", "install", "--dev"}, dB)
	if err == nil {
		t.Fatal("expected error installing --dev from a different checkout without --force")
	}
	if !strings.Contains(stdout.String(), "conflicted") {
		t.Errorf("expected output to report conflicted targets, got: %q", stdout.String())
	}

	linkPath := filepath.Join(claudeRoot, "README.md")
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink(%s): %v", linkPath, err)
	}
	if target != filepath.Join(bundleDirA, "README.md") {
		t.Errorf("expected symlink to remain pointed at repoA, got %q", target)
	}
}

func TestExecute_SkillsInstall_SwitchingBetweenProdAndDevRequiresForce(t *testing.T) {
	var stdout bytes.Buffer
	repo := testutil.TempRepo(t)
	writeDevBundle(t, repo)
	d := fakeSkillsDevDeps(t, &stdout, repo)

	if err := execute([]string{"skills", "install"}, d); err != nil {
		t.Fatalf("prod install: %v", err)
	}

	stdout.Reset()
	err := execute([]string{"skills", "install", "--dev"}, d)
	if err == nil {
		t.Fatal("expected switching to --dev without --force to conflict")
	}
	if !strings.Contains(stdout.String(), "conflicted") {
		t.Errorf("expected conflicted output, got: %q", stdout.String())
	}
}

func TestExecute_SkillsInstallDev_RefusesCheckoutMissingBundle(t *testing.T) {
	var stdout bytes.Buffer
	repo := testutil.TempRepo(t) // no skills/ directory written
	d := fakeSkillsDevDeps(t, &stdout, repo)

	if err := execute([]string{"skills", "install", "--dev"}, d); err == nil {
		t.Fatal("expected error for checkout missing gx's skill bundle")
	}
	manifestPath, _ := d.skillsManifestPath()
	if _, err := skills.Load(manifestPath); err == nil {
		t.Error("expected no manifest to be written for a refused dev install")
	}
}
