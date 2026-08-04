package skills

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestUninstallRemovesManifestOwnedPathsAndManifest(t *testing.T) {
	srcDir := t.TempDir()
	root := t.TempDir()
	manifestPath := filepath.Join(t.TempDir(), "skill.json")

	files := []SourceFile{writeSourceFile(t, srcDir, "SKILL.md", "hello")}
	if err := Install(manifestPath, InstallRequest{Source: "example/skill", AgentRoots: []string{root}, Files: files}); err != nil {
		t.Fatalf("Install: %v", err)
	}

	if err := Uninstall(manifestPath, ForcePolicy{}); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "SKILL.md")); !os.IsNotExist(err) {
		t.Errorf("SKILL.md still exists after uninstall: err = %v", err)
	}
	if _, err := Load(manifestPath); !errors.Is(err, ErrNotExist) {
		t.Errorf("Load err = %v, want ErrNotExist after full uninstall", err)
	}
}

func TestUninstallPreservesUnrelatedFilesAndDirectories(t *testing.T) {
	srcDir := t.TempDir()
	root := t.TempDir()
	manifestPath := filepath.Join(t.TempDir(), "skill.json")

	files := []SourceFile{writeSourceFile(t, srcDir, "managed/SKILL.md", "hello")}
	if err := Install(manifestPath, InstallRequest{Source: "example/skill", AgentRoots: []string{root}, Files: files}); err != nil {
		t.Fatalf("Install: %v", err)
	}

	unrelatedDir := filepath.Join(root, "managed")
	unrelatedPath := filepath.Join(unrelatedDir, "unrelated.md")
	if err := os.WriteFile(unrelatedPath, []byte("keep me"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := Uninstall(manifestPath, ForcePolicy{}); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	if got := readFile(t, unrelatedPath); got != "keep me" {
		t.Errorf("unrelated.md content = %q, want keep me", got)
	}
	if _, err := os.Stat(unrelatedDir); err != nil {
		t.Errorf("unrelated directory removed: %v", err)
	}
}

func TestUninstallPreservesLocallyModifiedFilesWithoutForce(t *testing.T) {
	srcDir := t.TempDir()
	root := t.TempDir()
	manifestPath := filepath.Join(t.TempDir(), "skill.json")

	files := []SourceFile{
		writeSourceFile(t, srcDir, "SKILL.md", "hello"),
		writeSourceFile(t, srcDir, "scripts/run.sh", "echo hi"),
	}
	if err := Install(manifestPath, InstallRequest{Source: "example/skill", AgentRoots: []string{root}, Files: files}); err != nil {
		t.Fatalf("Install: %v", err)
	}

	modifiedPath := filepath.Join(root, "SKILL.md")
	if err := os.WriteFile(modifiedPath, []byte("locally edited"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := Uninstall(manifestPath, ForcePolicy{}); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	if got := readFile(t, modifiedPath); got != "locally edited" {
		t.Errorf("SKILL.md content = %q, want preserved local edit", got)
	}
	if _, err := os.Stat(filepath.Join(root, "scripts/run.sh")); !os.IsNotExist(err) {
		t.Errorf("scripts/run.sh still exists after uninstall: err = %v", err)
	}

	m, err := Load(manifestPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(m.Files) != 1 || m.Files[0].Path != "SKILL.md" {
		t.Fatalf("manifest Files = %+v, want only SKILL.md preserved", m.Files)
	}
}

func TestUninstallForceRemovesModifiedFiles(t *testing.T) {
	srcDir := t.TempDir()
	root := t.TempDir()
	manifestPath := filepath.Join(t.TempDir(), "skill.json")

	files := []SourceFile{writeSourceFile(t, srcDir, "SKILL.md", "hello")}
	if err := Install(manifestPath, InstallRequest{Source: "example/skill", AgentRoots: []string{root}, Files: files}); err != nil {
		t.Fatalf("Install: %v", err)
	}

	modifiedPath := filepath.Join(root, "SKILL.md")
	if err := os.WriteFile(modifiedPath, []byte("locally edited"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := Uninstall(manifestPath, NewForcePolicy("SKILL.md")); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	if _, err := os.Stat(modifiedPath); !os.IsNotExist(err) {
		t.Errorf("SKILL.md still exists after forced uninstall: err = %v", err)
	}
	if _, err := Load(manifestPath); !errors.Is(err, ErrNotExist) {
		t.Errorf("Load err = %v, want ErrNotExist after full forced uninstall", err)
	}
}

func TestUninstallOnMissingManifestReturnsErrNotExist(t *testing.T) {
	manifestPath := filepath.Join(t.TempDir(), "missing.json")

	err := Uninstall(manifestPath, ForcePolicy{})
	if !errors.Is(err, ErrNotExist) {
		t.Fatalf("Uninstall err = %v, want ErrNotExist", err)
	}
}
