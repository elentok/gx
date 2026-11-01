package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyPlan_TablePrintsEstimates(t *testing.T) {
	out, err := runIn(t, t.TempDir(), "tickets", "verify-plan", "--table")
	if err != nil {
		t.Fatalf("--table: %v", err)
	}
	wantLines(t, out, "web flow", "6K", "CLI check", "3K", "Go test run", "4K", "skill quote", "2K",
		"mock state", "8K", "part base", "15K", "collect estimate", "30K", "split limit", "60K")
}

func TestVerifyPlan_PrintsEntries(t *testing.T) {
	dir := t.TempDir()
	plan := filepath.Join(dir, "plan.yaml")
	if err := os.WriteFile(plan, []byte("stories:\n  - story: 1\n    web_flows: 8\n  - story: 2\n    web_flows: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runIn(t, dir, "tickets", "verify-plan", "mine:epic", "--plan", plan)
	if err != nil {
		t.Fatalf("verify-plan: %v", err)
	}
	wantLines(t, out, "verify-part-1: window=63K", "over budget", "verify-part-2:", "verify-collect: window=30K blocked_by=verify-part-1,verify-part-2")
}

func TestVerifyPlan_BadPlanFails(t *testing.T) {
	dir := t.TempDir()
	plan := filepath.Join(dir, "plan.yaml")
	if err := os.WriteFile(plan, []byte("stories: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runIn(t, dir, "tickets", "verify-plan", "mine:epic", "--plan", plan); err == nil {
		t.Error("empty plan: want error")
	}
	if _, err := runIn(t, dir, "tickets", "verify-plan", "mine:epic", "--plan", filepath.Join(dir, "missing.yaml")); err == nil {
		t.Error("missing plan: want error")
	}
	if _, err := runIn(t, dir, "tickets", "verify-plan", "mine:epic"); err == nil {
		t.Error("no --plan: want error")
	}
}
