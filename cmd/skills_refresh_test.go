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

func statusPathOf(t *testing.T, d deps) string {
	t.Helper()
	path, err := d.skillsStatusPath()
	if err != nil {
		t.Fatalf("skillsStatusPath: %v", err)
	}
	return path
}

func manifestOf(t *testing.T, d deps) skills.Manifest {
	t.Helper()
	path, _ := d.skillsManifestPath()
	m, err := skills.Load(path)
	if err != nil {
		t.Fatalf("Load manifest: %v", err)
	}
	return m
}

func TestRefreshSkills_NoManifestInstallsManagedCopies(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	d := fakeSkillsDeps(t, &out)
	claudeRoot, _ := agentRootsOf(t, d)

	refreshSkills(d, &out)

	if _, err := os.Stat(filepath.Join(claudeRoot, "gx.md")); err != nil {
		t.Errorf("gx.md not installed: %v", err)
	}
	if m := manifestOf(t, d); m.Mode != skills.ModeManagedCopy {
		t.Errorf("manifest mode = %q, want %q", m.Mode, skills.ModeManagedCopy)
	}
	if !strings.Contains(out.String(), "skills: ") || !strings.Contains(out.String(), " installed") {
		t.Errorf("output = %q, want an install summary", out.String())
	}
	if p := skills.ReadProblem(statusPathOf(t, d)); p != "" {
		t.Errorf("recorded problem = %q, want none", p)
	}
}

func TestRefreshSkills_UpToDateSaysSo(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	d := fakeSkillsDeps(t, &out)
	refreshSkills(d, &out)
	out.Reset()

	refreshSkills(d, &out)

	if out.String() != "skills: up to date\n" {
		t.Errorf("output = %q, want %q", out.String(), "skills: up to date\n")
	}
}

func TestRefreshSkills_DevInstallRelinksFromSameCheckout(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	repo := testutil.TempRepo(t)
	bundleDir := writeDevBundle(t, repo)
	d := fakeSkillsDevDeps(t, &out, repo)
	claudeRoot, _ := agentRootsOf(t, d)
	if err := execute([]string{"skills", "install", "--dev"}, d); err != nil {
		t.Fatalf("skills install --dev: %v", err)
	}
	link := filepath.Join(claudeRoot, "gx-implement", "SKILL.md")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	// The start runs from anywhere, not from the checkout.
	d.getwd = func() (string, error) { return t.TempDir(), nil }
	out.Reset()

	refreshSkills(d, &out)

	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink: %v (output %q)", err, out.String())
	}
	if want := filepath.Join(bundleDir, "gx-implement", "SKILL.md"); target != want {
		t.Errorf("link target = %q, want %q", target, want)
	}
	if m := manifestOf(t, d); m.Mode != skills.ModeSymlink {
		t.Errorf("manifest mode = %q, want %q", m.Mode, skills.ModeSymlink)
	}
	if p := skills.ReadProblem(statusPathOf(t, d)); p != "" {
		t.Errorf("recorded problem = %q, want none", p)
	}
}

func TestRefreshSkills_MissingDevCheckoutWarnsAndLeavesSkills(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	repo := testutil.TempRepo(t)
	writeDevBundle(t, repo)
	d := fakeSkillsDevDeps(t, &out, repo)
	if err := execute([]string{"skills", "install", "--dev"}, d); err != nil {
		t.Fatalf("skills install --dev: %v", err)
	}
	before := manifestOf(t, d)
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}
	out.Reset()

	refreshSkills(d, &out)

	if !strings.Contains(out.String(), "warning: skills: dev checkout "+repo+" is gone") {
		t.Errorf("output = %q, want a missing-checkout warning", out.String())
	}
	if p := skills.ReadProblem(statusPathOf(t, d)); !strings.Contains(p, "is gone") {
		t.Errorf("recorded problem = %q, want the missing-checkout warning", p)
	}
	if after := manifestOf(t, d); after.Mode != before.Mode || after.Source != before.Source {
		t.Errorf("manifest changed: %+v -> %+v", before, after)
	}
}

func TestRefreshSkills_ConflictRecordsProblemAndCleanInstallClearsIt(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	d := fakeSkillsDeps(t, &out)
	claudeRoot, _ := agentRootsOf(t, d)
	if err := os.MkdirAll(claudeRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.WriteFile(t, claudeRoot, "gx.md", "not gx's")

	refreshSkills(d, &out)

	conflicted := filepath.Join(claudeRoot, "gx.md")
	if !strings.Contains(out.String(), "warning: skills: install refused") || !strings.Contains(out.String(), conflicted) {
		t.Errorf("output = %q, want a conflict warning naming %s", out.String(), conflicted)
	}
	problem := skills.ReadProblem(statusPathOf(t, d))
	if !strings.Contains(problem, conflicted) || !strings.Contains(problem, "gx skills install --force <path>") {
		t.Errorf("recorded problem = %q, want the path and the fix", problem)
	}

	if err := execute([]string{"skills", "install", "--force-all"}, d); err != nil {
		t.Fatalf("skills install --force-all: %v", err)
	}
	if p := skills.ReadProblem(statusPathOf(t, d)); p != "" {
		t.Errorf("recorded problem after a clean install = %q, want none", p)
	}
}
