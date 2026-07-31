package herdr

import (
	"errors"
	"testing"
)

func TestFindWorkspace_ReturnsMatchingID(t *testing.T) {
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		return []byte(`{"result":{"workspaces":[
			{"workspace_id":"w1","label":"other"},
			{"workspace_id":"w2","label":"my-epic"}
		]}}`), nil
	})
	id, err := FindWorkspace("my-epic")
	if err != nil {
		t.Fatalf("FindWorkspace() error = %v", err)
	}
	if id != "w2" {
		t.Fatalf("FindWorkspace() = %q, want %q", id, "w2")
	}
}

func TestFindWorkspace_NoMatch_ReturnsEmpty(t *testing.T) {
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		return []byte(`{"result":{"workspaces":[{"workspace_id":"w1","label":"other"}]}}`), nil
	})
	id, err := FindWorkspace("my-epic")
	if err != nil {
		t.Fatalf("FindWorkspace() error = %v", err)
	}
	if id != "" {
		t.Fatalf("FindWorkspace() = %q, want empty", id)
	}
}

func TestFindWorkspace_ListError_Propagates(t *testing.T) {
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		return nil, errors.New("boom")
	})
	if _, err := FindWorkspace("my-epic"); err == nil {
		t.Fatal("FindWorkspace() error = nil, want error")
	}
}

func TestFindOrCreateWorkspace_Existing_Focuses(t *testing.T) {
	var calls [][]string
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		calls = append(calls, args)
		if args[0] == "workspace" && args[1] == "list" {
			return []byte(`{"result":{"workspaces":[{"workspace_id":"w2","label":"my-epic"}]}}`), nil
		}
		return []byte(`{"result":{"type":"workspace_focused","workspace_id":"w2"}}`), nil
	})
	id, err := FindOrCreateWorkspace("my-epic", "/path")
	if err != nil {
		t.Fatalf("FindOrCreateWorkspace() error = %v", err)
	}
	if id != "w2" {
		t.Fatalf("FindOrCreateWorkspace() = %q, want %q", id, "w2")
	}
	if len(calls) != 2 || calls[1][0] != "workspace" || calls[1][1] != "focus" || calls[1][2] != "w2" {
		t.Fatalf("expected a workspace focus call, got calls = %v", calls)
	}
}

func TestFindOrCreateWorkspace_Missing_Creates(t *testing.T) {
	var calls [][]string
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		calls = append(calls, args)
		if args[0] == "workspace" && args[1] == "list" {
			return []byte(`{"result":{"workspaces":[]}}`), nil
		}
		return []byte(`{"result":{"workspace":{"workspace_id":"w9"}}}`), nil
	})
	id, err := FindOrCreateWorkspace("my-epic", "/path")
	if err != nil {
		t.Fatalf("FindOrCreateWorkspace() error = %v", err)
	}
	if id != "w9" {
		t.Fatalf("FindOrCreateWorkspace() = %q, want %q", id, "w9")
	}
	if len(calls) != 2 || calls[1][0] != "workspace" || calls[1][1] != "create" {
		t.Fatalf("expected a workspace create call, got calls = %v", calls)
	}
}
