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

	err := Install(manifestPath, InstallRequest{
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

	if err := Install(manifestPath, req); err != nil {
		t.Fatalf("first Install: %v", err)
	}
	if err := Install(manifestPath, req); err != nil {
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
	if err := Install(manifestPath, InstallRequest{
		Source: "example/skill", AgentRoots: []string{root}, Files: []SourceFile{original},
	}); err != nil {
		t.Fatalf("initial Install: %v", err)
	}

	upgraded := writeSourceFile(t, srcDir, "SKILL.md", "v2")
	if err := Install(manifestPath, InstallRequest{
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
	if err := Install(manifestPath, InstallRequest{
		Source: "example/skill", AgentRoots: []string{root}, Files: []SourceFile{original},
	}); err != nil {
		t.Fatalf("initial Install: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("locally edited"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	upgraded := writeSourceFile(t, srcDir, "SKILL.md", "v2")
	err := Install(manifestPath, InstallRequest{
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
	err := Install(manifestPath, InstallRequest{Source: "example/skill", AgentRoots: []string{root}, Files: files})

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
	err := Install(manifestPath, InstallRequest{
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
	err := Install(manifestPath, InstallRequest{
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
