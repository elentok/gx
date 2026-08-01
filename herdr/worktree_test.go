package herdr

import (
	"errors"
	"strings"
	"testing"
)

func TestWorktreeCreate_ParsesResult(t *testing.T) {
	var gotArgs []string
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		gotArgs = args
		return []byte(`{"result":{
			"type":"worktree_created",
			"workspace":{"workspace_id":"w9"},
			"tab":{"tab_id":"w9:t1"},
			"root_pane":{"pane_id":"w9:p1"},
			"worktree":{"path":"/repo/iter-01","branch":"ralph-loop/iter-01","label":"iter-01"}
		}}`), nil
	})
	wt, err := WorktreeCreate(WorktreeCreateOptions{
		WorkspaceID: "wE",
		Cwd:         "/repo",
		Branch:      "ralph-loop/iter-01",
		Base:        "feature",
		Label:       "iter-01",
		Focus:       true,
	})
	if err != nil {
		t.Fatalf("WorktreeCreate() error = %v", err)
	}
	want := Worktree{
		WorkspaceID: "w9",
		TabID:       "w9:t1",
		PaneID:      "w9:p1",
		Path:        "/repo/iter-01",
		Branch:      "ralph-loop/iter-01",
		Label:       "iter-01",
	}
	if wt != want {
		t.Fatalf("WorktreeCreate() = %+v, want %+v", wt, want)
	}
	argLine := strings.Join(gotArgs, " ")
	for _, want := range []string{"worktree create", "--workspace wE", "--branch ralph-loop/iter-01", "--base feature", "--label iter-01", "--focus"} {
		if !strings.Contains(argLine, want) {
			t.Errorf("args %q missing %q", argLine, want)
		}
	}
	if strings.Contains(argLine, "--cwd") {
		t.Errorf("args %q should not include --cwd when --workspace is set", argLine)
	}
}

func TestWorktreeCreate_NoWorkspace_UsesCwd(t *testing.T) {
	var gotArgs []string
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		gotArgs = args
		return []byte(`{"result":{
			"type":"worktree_created",
			"workspace":{"workspace_id":"w9"},
			"tab":{"tab_id":"w9:t1"},
			"root_pane":{"pane_id":"w9:p1"},
			"worktree":{"path":"/repo/iter-01","branch":"ralph-loop/iter-01","label":"iter-01"}
		}}`), nil
	})
	if _, err := WorktreeCreate(WorktreeCreateOptions{Cwd: "/repo", Branch: "ralph-loop/iter-01"}); err != nil {
		t.Fatalf("WorktreeCreate() error = %v", err)
	}
	argLine := strings.Join(gotArgs, " ")
	if !strings.Contains(argLine, "--cwd /repo") {
		t.Errorf("args %q missing --cwd /repo", argLine)
	}
	if strings.Contains(argLine, "--workspace") {
		t.Errorf("args %q should not include --workspace when WorkspaceID is empty", argLine)
	}
}

func TestWorktreeCreate_CommandError_Propagates(t *testing.T) {
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		return []byte("boom"), errors.New("exit 1")
	})
	if _, err := WorktreeCreate(WorktreeCreateOptions{Cwd: "/repo"}); err == nil {
		t.Fatal("WorktreeCreate() error = nil, want error")
	}
}

func TestWorktreeOpen_ParsesAlreadyOpen(t *testing.T) {
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		return []byte(`{"result":{
			"type":"worktree_opened",
			"already_open":true,
			"workspace":{"workspace_id":"w9"},
			"tab":{"tab_id":"w9:t1"},
			"root_pane":{"pane_id":"w9:p1"},
			"worktree":{"path":"/repo/iter-01","branch":"ralph-loop/iter-01","label":"iter-01"}
		}}`), nil
	})
	wt, err := WorktreeOpen(WorktreeOpenOptions{Cwd: "/repo", Path: "/repo/iter-01"})
	if err != nil {
		t.Fatalf("WorktreeOpen() error = %v", err)
	}
	if !wt.AlreadyOpen {
		t.Fatalf("WorktreeOpen().AlreadyOpen = false, want true")
	}
	if wt.WorkspaceID != "w9" {
		t.Fatalf("WorktreeOpen().WorkspaceID = %q, want %q", wt.WorkspaceID, "w9")
	}
}

func TestWorktreeRemove_BuildsArgs(t *testing.T) {
	var gotArgs []string
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		gotArgs = args
		return []byte(`{"result":{"type":"worktree_removed","workspace_id":"w9","path":"/repo/iter-01","forced":true}}`), nil
	})
	if err := WorktreeRemove("w9", true); err != nil {
		t.Fatalf("WorktreeRemove() error = %v", err)
	}
	want := []string{"worktree", "remove", "--workspace", "w9", "--force"}
	if strings.Join(gotArgs, " ") != strings.Join(want, " ") {
		t.Fatalf("WorktreeRemove() args = %v, want %v", gotArgs, want)
	}
}

func TestWorktreeRemove_NoForce_OmitsFlag(t *testing.T) {
	var gotArgs []string
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		gotArgs = args
		return []byte(`{"result":{"type":"worktree_removed","workspace_id":"w9","path":"/repo/iter-01","forced":false}}`), nil
	})
	if err := WorktreeRemove("w9", false); err != nil {
		t.Fatalf("WorktreeRemove() error = %v", err)
	}
	for _, a := range gotArgs {
		if a == "--force" {
			t.Fatalf("WorktreeRemove() args = %v, want no --force", gotArgs)
		}
	}
}

func TestWorktreeRemove_CommandError_Propagates(t *testing.T) {
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		return []byte("not found"), errors.New("exit 1")
	})
	if err := WorktreeRemove("w9", false); err == nil {
		t.Fatal("WorktreeRemove() error = nil, want error")
	}
}
