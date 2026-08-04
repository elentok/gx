package skills

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeSourceFile(t *testing.T, dir, relPath, content string) SourceFile {
	t.Helper()
	abs := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(abs, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return SourceFile{RelPath: relPath, AbsPath: abs}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	return string(data)
}

func TestInstallCreatesAgentRootsAndCopiesFiles(t *testing.T) {
	srcDir := t.TempDir()
	root := filepath.Join(t.TempDir(), "does", "not", "exist", "yet")
	manifestPath := filepath.Join(t.TempDir(), "skill.json")

	files := []SourceFile{
		writeSourceFile(t, srcDir, "SKILL.md", "hello"),
		writeSourceFile(t, srcDir, "scripts/run.sh", "echo hi"),
	}

	_, err := Install(manifestPath, InstallRequest{
		Source:     "example/skill",
		AgentRoots: []string{root},
		Files:      files,
	})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if got := readFile(t, filepath.Join(root, "SKILL.md")); got != "hello" {
		t.Errorf("SKILL.md content = %q, want hello", got)
	}
	if got := readFile(t, filepath.Join(root, "scripts/run.sh")); got != "echo hi" {
		t.Errorf("scripts/run.sh content = %q, want %q", got, "echo hi")
	}

	m, err := Load(manifestPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m.Mode != ModeManagedCopy || m.Source != "example/skill" {
		t.Errorf("manifest Mode/Source = %v/%v, want managed-copy/example/skill", m.Mode, m.Source)
	}
	if len(m.Files) != 2 {
		t.Fatalf("manifest Files = %+v, want 2 entries", m.Files)
	}
}

func TestReinstallingUnchangedContentIsIdempotent(t *testing.T) {
	srcDir := t.TempDir()
	root := t.TempDir()
	manifestPath := filepath.Join(t.TempDir(), "skill.json")

	files := []SourceFile{writeSourceFile(t, srcDir, "SKILL.md", "hello")}
	req := InstallRequest{Source: "example/skill", AgentRoots: []string{root}, Files: files}

	if _, err := Install(manifestPath, req); err != nil {
		t.Fatalf("first Install: %v", err)
	}
	if _, err := Install(manifestPath, req); err != nil {
		t.Fatalf("second Install: %v", err)
	}

	if got := readFile(t, filepath.Join(root, "SKILL.md")); got != "hello" {
		t.Errorf("SKILL.md content = %q, want hello", got)
	}
}

func TestUpgradeReplacesContentMatchingPriorManifest(t *testing.T) {
	srcDir := t.TempDir()
	root := t.TempDir()
	manifestPath := filepath.Join(t.TempDir(), "skill.json")

	original := writeSourceFile(t, srcDir, "SKILL.md", "v1")
	if _, err := Install(manifestPath, InstallRequest{
		Source: "example/skill", AgentRoots: []string{root}, Files: []SourceFile{original},
	}); err != nil {
		t.Fatalf("initial Install: %v", err)
	}

	upgraded := writeSourceFile(t, srcDir, "SKILL.md", "v2")
	if _, err := Install(manifestPath, InstallRequest{
		Source: "example/skill", AgentRoots: []string{root}, Files: []SourceFile{upgraded},
	}); err != nil {
		t.Fatalf("upgrade Install: %v", err)
	}

	if got := readFile(t, filepath.Join(root, "SKILL.md")); got != "v2" {
		t.Errorf("SKILL.md content = %q, want v2", got)
	}
}

func TestInstallRefusesLocallyModifiedContent(t *testing.T) {
	srcDir := t.TempDir()
	root := t.TempDir()
	manifestPath := filepath.Join(t.TempDir(), "skill.json")

	original := writeSourceFile(t, srcDir, "SKILL.md", "v1")
	if _, err := Install(manifestPath, InstallRequest{
		Source: "example/skill", AgentRoots: []string{root}, Files: []SourceFile{original},
	}); err != nil {
		t.Fatalf("initial Install: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("locally edited"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	upgraded := writeSourceFile(t, srcDir, "SKILL.md", "v2")
	_, err := Install(manifestPath, InstallRequest{
		Source: "example/skill", AgentRoots: []string{root}, Files: []SourceFile{upgraded},
	})
	var conflictErr *ConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("Install err = %v, want *ConflictError", err)
	}
	if conflictErr.Conflicts["SKILL.md"] != OwnershipModified {
		t.Errorf("Conflicts[SKILL.md] = %v, want OwnershipModified", conflictErr.Conflicts["SKILL.md"])
	}
	if got := readFile(t, filepath.Join(root, "SKILL.md")); got != "locally edited" {
		t.Errorf("SKILL.md content = %q, want unchanged local edit", got)
	}
}

func TestInstallRefusesUnrelatedCollision(t *testing.T) {
	srcDir := t.TempDir()
	root := t.TempDir()
	manifestPath := filepath.Join(t.TempDir(), "skill.json")

	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("not gx's"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	files := []SourceFile{writeSourceFile(t, srcDir, "SKILL.md", "hello")}
	_, err := Install(manifestPath, InstallRequest{Source: "example/skill", AgentRoots: []string{root}, Files: files})

	var conflictErr *ConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("Install err = %v, want *ConflictError", err)
	}
	if conflictErr.Conflicts["SKILL.md"] != OwnershipUnrelatedCollision {
		t.Errorf("Conflicts[SKILL.md] = %v, want OwnershipUnrelatedCollision", conflictErr.Conflicts["SKILL.md"])
	}
	if _, err := Load(manifestPath); !errors.Is(err, ErrNotExist) {
		t.Errorf("Load err = %v, want ErrNotExist (refused install must not publish a manifest)", err)
	}
}

func TestForcedInstallReplacesExplicitConflictOnly(t *testing.T) {
	srcDir := t.TempDir()
	root := t.TempDir()
	manifestPath := filepath.Join(t.TempDir(), "skill.json")

	if err := os.WriteFile(filepath.Join(root, "unrelated.md"), []byte("not gx's"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	files := []SourceFile{
		writeSourceFile(t, srcDir, "SKILL.md", "hello"),
		writeSourceFile(t, srcDir, "unrelated.md", "gx wants this now"),
	}
	_, err := Install(manifestPath, InstallRequest{
		Source:     "example/skill",
		AgentRoots: []string{root},
		Files:      files,
		Force:      NewForcePolicy("unrelated.md"),
	})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if got := readFile(t, filepath.Join(root, "unrelated.md")); got != "gx wants this now" {
		t.Errorf("unrelated.md content = %q, want forced overwrite", got)
	}
	if got := readFile(t, filepath.Join(root, "SKILL.md")); got != "hello" {
		t.Errorf("SKILL.md content = %q, want hello", got)
	}
}

func TestInstallSymlinkModeLinksToSourceAndRecordsLinkTarget(t *testing.T) {
	srcDir := t.TempDir()
	root := t.TempDir()
	manifestPath := filepath.Join(t.TempDir(), "skill.json")

	src := writeSourceFile(t, srcDir, "SKILL.md", "hello")
	_, err := Install(manifestPath, InstallRequest{
		Source:     "dev checkout",
		AgentRoots: []string{root},
		Files:      []SourceFile{src},
		Mode:       ModeSymlink,
	})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	target := filepath.Join(root, "SKILL.md")
	got, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if got != src.AbsPath {
		t.Errorf("symlink target = %q, want %q", got, src.AbsPath)
	}

	m, err := Load(manifestPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m.Mode != ModeSymlink {
		t.Errorf("manifest Mode = %v, want %v", m.Mode, ModeSymlink)
	}
	if len(m.Files) != 1 || m.Files[0].LinkTarget != src.AbsPath || m.Files[0].Hash != "" {
		t.Errorf("manifest Files = %+v, want a single entry with LinkTarget=%q and no Hash", m.Files, src.AbsPath)
	}
}

func TestInstallSymlinkModeReinstallFromSameSourceIsIdempotent(t *testing.T) {
	srcDir := t.TempDir()
	root := t.TempDir()
	manifestPath := filepath.Join(t.TempDir(), "skill.json")

	src := writeSourceFile(t, srcDir, "SKILL.md", "hello")
	req := InstallRequest{Source: "dev checkout", AgentRoots: []string{root}, Files: []SourceFile{src}, Mode: ModeSymlink}

	if _, err := Install(manifestPath, req); err != nil {
		t.Fatalf("first Install: %v", err)
	}
	if _, err := Install(manifestPath, req); err != nil {
		t.Fatalf("second Install: %v", err)
	}

	got, err := os.Readlink(filepath.Join(root, "SKILL.md"))
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if got != src.AbsPath {
		t.Errorf("symlink target = %q, want %q", got, src.AbsPath)
	}
}

func TestInstallSymlinkModeFromDifferentSourceRefusesToRetarget(t *testing.T) {
	srcDirA := t.TempDir()
	srcDirB := t.TempDir()
	root := t.TempDir()
	manifestPath := filepath.Join(t.TempDir(), "skill.json")

	srcA := writeSourceFile(t, srcDirA, "SKILL.md", "hello")
	if _, err := Install(manifestPath, InstallRequest{
		Source: "checkout A", AgentRoots: []string{root}, Files: []SourceFile{srcA}, Mode: ModeSymlink,
	}); err != nil {
		t.Fatalf("initial Install: %v", err)
	}

	srcB := writeSourceFile(t, srcDirB, "SKILL.md", "hello")
	_, err := Install(manifestPath, InstallRequest{
		Source: "checkout B", AgentRoots: []string{root}, Files: []SourceFile{srcB}, Mode: ModeSymlink,
	})
	var conflictErr *ConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("Install err = %v, want *ConflictError", err)
	}
	if conflictErr.Conflicts["SKILL.md"] != OwnershipWrongSymlinkTarget {
		t.Errorf("Conflicts[SKILL.md] = %v, want OwnershipWrongSymlinkTarget", conflictErr.Conflicts["SKILL.md"])
	}

	got, err := os.Readlink(filepath.Join(root, "SKILL.md"))
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if got != srcA.AbsPath {
		t.Errorf("symlink target = %q, want unchanged %q (checkout A)", got, srcA.AbsPath)
	}
}

func TestInstallSwitchingModesRequiresForce(t *testing.T) {
	srcDir := t.TempDir()
	root := t.TempDir()
	manifestPath := filepath.Join(t.TempDir(), "skill.json")

	src := writeSourceFile(t, srcDir, "SKILL.md", "hello")
	if _, err := Install(manifestPath, InstallRequest{
		Source: "example/skill", AgentRoots: []string{root}, Files: []SourceFile{src},
	}); err != nil {
		t.Fatalf("managed-copy Install: %v", err)
	}

	_, err := Install(manifestPath, InstallRequest{
		Source: "dev checkout", AgentRoots: []string{root}, Files: []SourceFile{src}, Mode: ModeSymlink,
	})
	var conflictErr *ConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("Install err = %v, want *ConflictError switching managed-copy -> symlink", err)
	}

	if _, err := Install(manifestPath, InstallRequest{
		Source: "dev checkout", AgentRoots: []string{root}, Files: []SourceFile{src}, Mode: ModeSymlink,
		Force: NewForcePolicy("SKILL.md"),
	}); err != nil {
		t.Fatalf("forced switch to symlink mode: %v", err)
	}
	if _, err := os.Readlink(filepath.Join(root, "SKILL.md")); err != nil {
		t.Fatalf("expected SKILL.md to become a symlink after forced switch: %v", err)
	}

	_, err = Install(manifestPath, InstallRequest{
		Source: "example/skill", AgentRoots: []string{root}, Files: []SourceFile{src},
	})
	if !errors.As(err, &conflictErr) {
		t.Fatalf("Install err = %v, want *ConflictError switching symlink -> managed-copy", err)
	}
}

func TestInstallFailureDoesNotPublishManifest(t *testing.T) {
	srcDir := t.TempDir()
	goodRoot := t.TempDir()
	badRoot := t.TempDir()
	manifestPath := filepath.Join(t.TempDir(), "skill.json")

	if err := os.Chmod(badRoot, 0500); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(badRoot, 0755) })

	files := []SourceFile{writeSourceFile(t, srcDir, "scripts/run.sh", "echo hi")}
	_, err := Install(manifestPath, InstallRequest{
		Source:     "example/skill",
		AgentRoots: []string{goodRoot, badRoot},
		Files:      files,
	})
	if err == nil {
		t.Fatal("Install: expected error from unwritable agent root, got nil")
	}
	var conflictErr *ConflictError
	if errors.As(err, &conflictErr) {
		t.Fatalf("Install err = %v, want a copy failure, not a conflict", err)
	}

	if _, err := Load(manifestPath); !errors.Is(err, ErrNotExist) {
		t.Errorf("Load err = %v, want ErrNotExist (failed install must not publish a manifest)", err)
	}
}
