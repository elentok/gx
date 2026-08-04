package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func targetStatusFor(t *testing.T, targets []Target, root, path string) TargetStatus {
	t.Helper()
	for _, tg := range targets {
		if tg.Root == root && tg.Path == path {
			return tg.Status
		}
	}
	t.Fatalf("no target for root=%s path=%s in %+v", root, path, targets)
	return ""
}

func TestPlanReportsInstalledUpdatedSkippedAndConflicted(t *testing.T) {
	srcDir := t.TempDir()
	root := t.TempDir()
	manifestPath := filepath.Join(t.TempDir(), "skill.json")

	unchanged := writeSourceFile(t, srcDir, "unchanged.md", "same")
	toUpgrade := writeSourceFile(t, srcDir, "upgrade.md", "v1")
	req := InstallRequest{Source: "example/skill", AgentRoots: []string{root}, Files: []SourceFile{unchanged, toUpgrade}}
	if err := Install(manifestPath, req); err != nil {
		t.Fatalf("initial Install: %v", err)
	}

	locallyModified := filepath.Join(root, "collide.md")
	if err := os.WriteFile(locallyModified, []byte("not gx's"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	upgraded := writeSourceFile(t, srcDir, "upgrade.md", "v2")
	collide := writeSourceFile(t, srcDir, "collide.md", "gx wants this")
	nextReq := InstallRequest{
		Source:     "example/skill",
		AgentRoots: []string{root},
		Files:      []SourceFile{unchanged, upgraded, collide},
	}

	targets, err := Plan(manifestPath, nextReq)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if got := targetStatusFor(t, targets, root, "unchanged.md"); got != StatusSkipped {
		t.Errorf("unchanged.md status = %v, want %v", got, StatusSkipped)
	}
	if got := targetStatusFor(t, targets, root, "upgrade.md"); got != StatusUpdated {
		t.Errorf("upgrade.md status = %v, want %v", got, StatusUpdated)
	}
	if got := targetStatusFor(t, targets, root, "collide.md"); got != StatusConflicted {
		t.Errorf("collide.md status = %v, want %v", got, StatusConflicted)
	}
}

func TestPlanReportsInstalledForNewPath(t *testing.T) {
	srcDir := t.TempDir()
	root := t.TempDir()
	manifestPath := filepath.Join(t.TempDir(), "skill.json")

	files := []SourceFile{writeSourceFile(t, srcDir, "SKILL.md", "hello")}
	targets, err := Plan(manifestPath, InstallRequest{Source: "example/skill", AgentRoots: []string{root}, Files: files})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if got := targetStatusFor(t, targets, root, "SKILL.md"); got != StatusInstalled {
		t.Errorf("SKILL.md status = %v, want %v", got, StatusInstalled)
	}
}

func TestPlanUninstallReportsRemovedAndConflicted(t *testing.T) {
	srcDir := t.TempDir()
	root := t.TempDir()
	manifestPath := filepath.Join(t.TempDir(), "skill.json")

	files := []SourceFile{
		writeSourceFile(t, srcDir, "clean.md", "hello"),
		writeSourceFile(t, srcDir, "SKILL.md", "hello"),
	}
	if err := Install(manifestPath, InstallRequest{Source: "example/skill", AgentRoots: []string{root}, Files: files}); err != nil {
		t.Fatalf("Install: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("locally edited"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	targets, err := PlanUninstall(manifestPath, ForcePolicy{})
	if err != nil {
		t.Fatalf("PlanUninstall: %v", err)
	}
	if got := targetStatusFor(t, targets, root, "clean.md"); got != StatusRemoved {
		t.Errorf("clean.md status = %v, want %v", got, StatusRemoved)
	}
	if got := targetStatusFor(t, targets, root, "SKILL.md"); got != StatusConflicted {
		t.Errorf("SKILL.md status = %v, want %v", got, StatusConflicted)
	}
}
