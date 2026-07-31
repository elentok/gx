package herdr

import (
	"errors"
	"strings"
	"testing"
)

func TestTabCreate_ParsesResult(t *testing.T) {
	var gotArgs []string
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		gotArgs = args
		return []byte(`{"result":{
			"type":"tab_created",
			"tab":{"tab_id":"wE:t9","workspace_id":"wE","number":9,"label":"iter-01","focused":true,"pane_count":1,"agent_status":"idle"},
			"root_pane":{"pane_id":"wE:p1"}
		}}`), nil
	})
	got, err := TabCreate(TabCreateOptions{WorkspaceID: "wE", Cwd: "/repo/iter-01", Label: "iter-01", Focus: true})
	if err != nil {
		t.Fatalf("TabCreate() error = %v", err)
	}
	want := CreatedTab{
		Tab: Tab{
			TabID:       "wE:t9",
			WorkspaceID: "wE",
			Number:      9,
			Label:       "iter-01",
			Focused:     true,
			PaneCount:   1,
			AgentStatus: "idle",
		},
		RootPaneID: "wE:p1",
	}
	if got != want {
		t.Fatalf("TabCreate() = %+v, want %+v", got, want)
	}
	argLine := strings.Join(gotArgs, " ")
	for _, want := range []string{"tab create", "--workspace wE", "--cwd /repo/iter-01", "--label iter-01", "--focus"} {
		if !strings.Contains(argLine, want) {
			t.Errorf("args %q missing %q", argLine, want)
		}
	}
}

func TestTabCreate_CommandError_Propagates(t *testing.T) {
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		return []byte("boom"), errors.New("exit 1")
	})
	if _, err := TabCreate(TabCreateOptions{WorkspaceID: "wE"}); err == nil {
		t.Fatal("TabCreate() error = nil, want error")
	}
}

func TestTabList_ParsesResult(t *testing.T) {
	var gotArgs []string
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		gotArgs = args
		return []byte(`{"result":{"type":"tab_list","tabs":[
			{"tab_id":"wE:t1","workspace_id":"wE","number":1,"label":"main","focused":false,"pane_count":3,"agent_status":"idle"},
			{"tab_id":"wE:t9","workspace_id":"wE","number":9,"label":"iter-01","focused":true,"pane_count":1,"agent_status":"working"}
		]}}`), nil
	})
	tabs, err := TabList("wE")
	if err != nil {
		t.Fatalf("TabList() error = %v", err)
	}
	if len(tabs) != 2 {
		t.Fatalf("TabList() returned %d tabs, want 2", len(tabs))
	}
	if tabs[1].TabID != "wE:t9" || tabs[1].AgentStatus != "working" {
		t.Fatalf("TabList()[1] = %+v", tabs[1])
	}
	if strings.Join(gotArgs, " ") != "tab list --workspace wE" {
		t.Fatalf("TabList() args = %v", gotArgs)
	}
}

func TestTabList_NoWorkspace_OmitsFlag(t *testing.T) {
	var gotArgs []string
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		gotArgs = args
		return []byte(`{"result":{"type":"tab_list","tabs":[]}}`), nil
	})
	if _, err := TabList(""); err != nil {
		t.Fatalf("TabList() error = %v", err)
	}
	if strings.Join(gotArgs, " ") != "tab list" {
		t.Fatalf("TabList() args = %v, want [tab list]", gotArgs)
	}
}

func TestTabList_CommandError_Propagates(t *testing.T) {
	withFakeCommand(t, func(args ...string) ([]byte, error) {
		return nil, errors.New("boom")
	})
	if _, err := TabList("wE"); err == nil {
		t.Fatal("TabList() error = nil, want error")
	}
}
