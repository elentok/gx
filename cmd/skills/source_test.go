package skills_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeSource builds a source with skills alpha and beta, a loose file and a
// folder without SKILL.md.
func writeSource(t *testing.T) string {
	t.Helper()
	src := filepath.Join(t.TempDir(), "src")
	writeFile(t, filepath.Join(src, "alpha", "SKILL.md"), "alpha")
	writeFile(t, filepath.Join(src, "alpha", "ref", "notes.md"), "notes")
	writeFile(t, filepath.Join(src, "beta", "SKILL.md"), "beta")
	writeFile(t, filepath.Join(src, "README.md"), "loose")
	writeFile(t, filepath.Join(src, "docs", "page.md"), "no skill")
	return src
}

func manifestCount(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			n++
		}
	}
	return n
}

func TestSkillsInstallSource_LinksEachSkillIntoBothRoots(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	src := writeSource(t)

	if err := run(t, d, "install", src); err != nil {
		t.Fatalf("install: %v", err)
	}

	claude, codex := agentRootsOf(t, d)
	for _, root := range []string{claude, codex} {
		for _, name := range []string{"alpha", "beta"} {
			target, err := os.Readlink(filepath.Join(root, name))
			if err != nil {
				t.Fatalf("readlink %s/%s: %v", root, name, err)
			}
			if target != filepath.Join(src, name) {
				t.Errorf("%s link target = %s", name, target)
			}
		}
		if got, err := os.ReadFile(filepath.Join(root, "alpha", "ref", "notes.md")); err != nil || string(got) != "notes" {
			t.Errorf("nested file through link = %q, %v", got, err)
		}
		for _, name := range []string{"README.md", "docs"} {
			if _, err := os.Lstat(filepath.Join(root, name)); err == nil {
				t.Errorf("%s must not be installed under %s", name, root)
			}
		}
	}
	if !strings.Contains(stdout.String(), "2 skills") {
		t.Errorf("summary lacks skill count: %q", stdout.String())
	}
}

func TestSkillsInstallSource_RerunIsNoOp(t *testing.T) {
	t.Parallel()
	for _, extra := range [][]string{nil, {"--copy"}} {
		var stdout bytes.Buffer
		d := fakeSkillsDeps(t, &stdout)
		src := writeSource(t)
		args := append([]string{"install", src}, extra...)
		if err := run(t, d, args...); err != nil {
			t.Fatalf("install %v: %v", extra, err)
		}
		stdout.Reset()
		if err := run(t, d, args...); err != nil {
			t.Fatalf("rerun %v: %v", extra, err)
		}
		if !strings.Contains(stdout.String(), "skipped") || strings.Contains(stdout.String(), "installed") || strings.Contains(stdout.String(), "updated") {
			t.Errorf("rerun %v output: %q", extra, stdout.String())
		}
	}
}

func TestSkillsInstallSource_CopyMakesRealFolders(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	src := writeSource(t)

	if err := run(t, d, "install", src, "--copy"); err != nil {
		t.Fatalf("install --copy: %v", err)
	}

	claude, _ := agentRootsOf(t, d)
	info, err := os.Lstat(filepath.Join(claude, "alpha"))
	if err != nil || !info.IsDir() {
		t.Fatalf("alpha must be a real folder: %v %v", info, err)
	}
	if got, err := os.ReadFile(filepath.Join(claude, "alpha", "ref", "notes.md")); err != nil || string(got) != "notes" {
		t.Errorf("copied nested file = %q, %v", got, err)
	}
}

func TestSkillsInstallSource_SymlinkedSpellingIsOneRecord(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	src := writeSource(t)
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(src, alias); err != nil {
		t.Fatal(err)
	}
	manifestDir, _ := d.ManifestDir()

	if err := run(t, d, "install", src); err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := run(t, d, "install", alias); err != nil {
		t.Fatalf("install alias: %v", err)
	}
	if n := manifestCount(t, manifestDir); n != 1 {
		t.Errorf("manifest records = %d, want 1", n)
	}
}

func TestSkillsInstallSource_SymlinkedSkillFolderAndMarkerCount(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	src := writeSource(t)
	real := filepath.Join(t.TempDir(), "elsewhere")
	writeFile(t, filepath.Join(real, "SKILL.md"), "gamma")
	if err := os.Symlink(real, filepath.Join(src, "gamma")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(t.TempDir(), "x"), "")
	marker := filepath.Join(t.TempDir(), "marker.md")
	writeFile(t, marker, "delta")
	if err := os.MkdirAll(filepath.Join(src, "delta"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(marker, filepath.Join(src, "delta", "SKILL.md")); err != nil {
		t.Fatal(err)
	}

	if err := run(t, d, "install", src); err != nil {
		t.Fatalf("install: %v", err)
	}

	claude, _ := agentRootsOf(t, d)
	for _, name := range []string{"gamma", "delta"} {
		target, err := os.Readlink(filepath.Join(claude, name))
		if err != nil || target != filepath.Join(src, name) {
			t.Errorf("%s link = %q, %v", name, target, err)
		}
	}
}

func TestSkillsInstallSource_RefusesBadDirAndDevAndCopyWithoutDir(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	manifestDir, _ := d.ManifestDir()
	file := filepath.Join(t.TempDir(), "file")
	writeFile(t, file, "x")
	src := writeSource(t)

	for _, args := range [][]string{
		{"install", filepath.Join(t.TempDir(), "missing")},
		{"install", file},
		{"install", src, "--dev"},
		{"install", "--copy"},
	} {
		if err := run(t, d, args...); err == nil {
			t.Errorf("%v: want error", args)
		}
	}
	if n := manifestCount(t, manifestDir); n != 0 {
		t.Errorf("refused installs left %d records", n)
	}
}

func TestSkillsInstallSource_EmptySourceSaysSo(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	empty := t.TempDir()
	writeFile(t, filepath.Join(empty, "loose.md"), "x")
	manifestDir, _ := d.ManifestDir()

	if err := run(t, d, "install", empty); err != nil {
		t.Fatalf("install empty: %v", err)
	}
	if !strings.Contains(stdout.String(), "no skills found") {
		t.Errorf("output: %q", stdout.String())
	}
	if n := manifestCount(t, manifestDir); n != 0 {
		t.Errorf("empty source left %d records", n)
	}
}
