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
	link := filepath.Join(claudeRoot, "gx-implement")
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
	if want := filepath.Join(bundleDir, "gx-implement"); target != want {
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

func TestRefreshSkills_MigratesV1AndKeepsServerWarningOnConflict(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(map[bool]string{false: "migrates", true: "warns"}[conflict], func(t *testing.T) {
			var out bytes.Buffer
			repo := testutil.TempRepo(t)
			bundle := writeDevBundle(t, repo)
			d := fakeSkillsDevDeps(t, &out, repo)
			roots, _ := d.skillsAgentRoots()
			manifest, _ := d.skillsManifestPath()
			rels, err := skills.BundleFiles()
			if err != nil {
				t.Fatal(err)
			}
			m := skills.Manifest{SchemaVersion: 1, Mode: skills.ModeSymlink, Source: skills.DevSourcePrefix + repo, AgentRoots: roots}
			for _, rel := range rels {
				source := filepath.Join(bundle, rel)
				m.Files = append(m.Files, skills.ManagedFile{Path: rel, LinkTarget: source})
				for _, root := range roots {
					dest := filepath.Join(root, rel)
					if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(source, dest); err != nil {
						t.Fatal(err)
					}
				}
			}
			conflicting := filepath.Join(roots[0], "gx-implement", "personal.txt")
			if conflict {
				if err := os.WriteFile(conflicting, []byte("keep"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if err := skills.Save(manifest, m); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(manifest)
			if err != nil {
				t.Fatal(err)
			}
			d.getwd = func() (string, error) { t.Fatal("startup used invocation checkout"); return "", nil }
			refreshSkills(d, &out)
			if conflict {
				after, err := os.ReadFile(manifest)
				if err != nil {
					t.Fatal(err)
				}
				if string(after) != string(before) {
					t.Fatal("failed refresh changed manifest")
				}
				problem := skills.ReadProblem(statusPathOf(t, d))
				if !strings.Contains(problem, "gx-implement") || !strings.Contains(problem, "gx skills install --dev") {
					t.Fatal(problem)
				}
				if _, err := os.Stat(conflicting); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := os.Readlink(filepath.Join(roots[0], "gx-implement")); err != nil {
					t.Fatal(err)
				}
				if manifestOf(t, d).SchemaVersion != 2 {
					t.Fatal("legacy manifest not migrated")
				}
				if !strings.Contains(out.String(), "skills backups:") {
					t.Fatal("missing recovery location", out.String())
				}
			}
			sourceInfo, err := os.Lstat(filepath.Join(bundle, "gx-implement", "SKILL.md"))
			if err != nil || !sourceInfo.Mode().IsRegular() {
				t.Fatal("source changed", err)
			}
		})
	}
}
