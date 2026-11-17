package herdr

import (
	"slices"
	"testing"
)

func TestFindWorkspace_ReturnsMatchingID(t *testing.T) {
	cli, _ := stubHerdr(t, map[string]reply{"workspace list": {out: `{"result":{"workspaces":[
		{"workspace_id":"w1","label":"other"},
		{"workspace_id":"w2","label":"my-epic"}
	]}}`}})
	id, err := cli.FindWorkspace("my-epic")
	if err != nil {
		t.Fatalf("FindWorkspace() error = %v", err)
	}
	if id != "w2" {
		t.Fatalf("FindWorkspace() = %q, want %q", id, "w2")
	}
}

func TestFindWorkspace_NoMatch_ReturnsEmpty(t *testing.T) {
	cli, _ := stubHerdr(t, map[string]reply{"workspace list": {out: `{"result":{"workspaces":[{"workspace_id":"w1","label":"other"}]}}`}})
	id, err := cli.FindWorkspace("my-epic")
	if err != nil {
		t.Fatalf("FindWorkspace() error = %v", err)
	}
	if id != "" {
		t.Fatalf("FindWorkspace() = %q, want empty", id)
	}
}

func TestFindWorkspace_ListError_Propagates(t *testing.T) {
	cli, _ := stubHerdr(t, map[string]reply{"workspace list": {out: "boom", exit: 1}})
	if _, err := cli.FindWorkspace("my-epic"); err == nil {
		t.Fatal("FindWorkspace() error = nil, want error")
	}
}

func TestFindOrCreateWorkspace_Existing_Focuses(t *testing.T) {
	cli, argv := stubHerdr(t, map[string]reply{
		"workspace list":  {out: `{"result":{"workspaces":[{"workspace_id":"w2","label":"my-epic"}]}}`},
		"workspace focus": {out: `{"result":{"type":"workspace_focused","workspace_id":"w2"}}`},
	})
	id, err := cli.FindOrCreateWorkspace("my-epic", "/path")
	if err != nil {
		t.Fatalf("FindOrCreateWorkspace() error = %v", err)
	}
	if id != "w2" {
		t.Fatalf("FindOrCreateWorkspace() = %q, want %q", id, "w2")
	}
	if want := []string{"workspace list", "workspace focus w2"}; !slices.Equal(calls(t, argv), want) {
		t.Fatalf("calls = %v, want %v", calls(t, argv), want)
	}
}

func TestFindOrCreateWorkspace_Missing_Creates(t *testing.T) {
	cli, argv := stubHerdr(t, map[string]reply{
		"workspace list":   {out: `{"result":{"workspaces":[]}}`},
		"workspace create": {out: `{"result":{"workspace":{"workspace_id":"w9"}}}`},
	})
	id, err := cli.FindOrCreateWorkspace("my-epic", "/path")
	if err != nil {
		t.Fatalf("FindOrCreateWorkspace() error = %v", err)
	}
	if id != "w9" {
		t.Fatalf("FindOrCreateWorkspace() = %q, want %q", id, "w9")
	}
	if want := []string{"workspace list", "workspace create --cwd /path --label my-epic --focus"}; !slices.Equal(calls(t, argv), want) {
		t.Fatalf("calls = %v, want %v", calls(t, argv), want)
	}
}

func TestPing_Answers(t *testing.T) {
	cli, argv := stubHerdr(t, map[string]reply{"workspace list": {out: `{"result":{"workspaces":[]}}`}})
	if err := cli.Ping(); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
	if want := []string{"workspace list"}; !slices.Equal(calls(t, argv), want) {
		t.Fatalf("calls = %v, want %v", calls(t, argv), want)
	}
}

func TestPing_Fails(t *testing.T) {
	cli, _ := stubHerdr(t, map[string]reply{"workspace list": {out: "server not running", exit: 1}})
	if err := cli.Ping(); err == nil {
		t.Fatal("Ping() error = nil, want error")
	}
}
