package tickets

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/notify"
)

// runKey sends a key and feeds the command's message back once.
func runKey(t *testing.T, m QueueModel, k tea.KeyPressMsg) (QueueModel, tea.Msg) {
	t.Helper()
	next, cmd := m.Update(k)
	m = next.(QueueModel)
	if cmd == nil {
		return m, nil
	}
	msg := cmd()
	next, _ = m.Update(msg)
	return next.(QueueModel), msg
}

func harnessQueue(t *testing.T, api fakeServerAPI, project string) QueueModel {
	m := newMaintenanceQueue(t, api)
	m.projectFilter = project
	return m
}

func TestDefaultHarness_GlobalPickerSendsGlobalRequest(t *testing.T) {
	sets := &[]server.SetDefaultHarnessRequest{}
	m := harnessQueue(t, fakeServerAPI{
		harness:     server.DefaultHarnessStatus{Default: "claude"},
		harnessSets: sets,
		harnessRes:  server.SetDefaultHarnessResult{Notice: "switched"},
	}, "")

	m, _ = runKey(t, m, tea.KeyPressMsg{Code: 'H', Text: "H"})
	if !m.harnessPicker.open || len(m.harnessPicker.menu.Items) != 2 {
		t.Fatalf("global picker = %+v", m.harnessPicker)
	}
	if it, _ := m.harnessPicker.menu.Selected(); it.Value != "claude" {
		t.Fatalf("cursor starts on %q, want the current value claude", it.Value)
	}
	m, _ = runKey(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	m, msg := runKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})

	if m.harnessPicker.open {
		t.Fatal("picker still open after enter")
	}
	want := server.SetDefaultHarnessRequest{Scope: "global", Harness: "codex"}
	if len(*sets) != 1 || (*sets)[0] != want {
		t.Fatalf("requests = %+v, want %+v", *sets, want)
	}
	if _, ok := msg.(defaultHarnessWroteMsg); !ok {
		t.Fatalf("enter produced %T", msg)
	}
}

func TestDefaultHarness_ProjectPickerFollowGlobalSendsEmptyHarness(t *testing.T) {
	sets := &[]server.SetDefaultHarnessRequest{}
	m := harnessQueue(t, fakeServerAPI{
		harness: server.DefaultHarnessStatus{
			Default:  "claude",
			Projects: []server.ProjectDefaultHarness{{Project: "gx", Harness: "codex"}},
		},
		harnessSets: sets,
	}, "gx")

	m, _ = runKey(t, m, tea.KeyPressMsg{Code: 'H', Text: "H"})
	if len(m.harnessPicker.menu.Items) != 3 || m.harnessPicker.menu.Items[0].Label != "Follow global" {
		t.Fatalf("project picker items = %+v", m.harnessPicker.menu.Items)
	}
	if it, _ := m.harnessPicker.menu.Selected(); it.Value != "codex" {
		t.Fatalf("cursor starts on %q, want the override codex", it.Value)
	}
	if view := m.harnessPickerView(); !strings.Contains(view, "gx") {
		t.Fatalf("picker is not titled with the project:\n%s", view)
	}
	m, _ = runKey(t, m, tea.KeyPressMsg{Code: tea.KeyUp})
	m, _ = runKey(t, m, tea.KeyPressMsg{Code: tea.KeyUp})
	runKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})

	want := server.SetDefaultHarnessRequest{Scope: "project", Project: "gx", Harness: ""}
	if len(*sets) != 1 || (*sets)[0] != want {
		t.Fatalf("requests = %+v, want %+v", *sets, want)
	}
}

func TestDefaultHarness_RefusalShowsMessage(t *testing.T) {
	m := harnessQueue(t, fakeServerAPI{
		harness:     server.DefaultHarnessStatus{Default: "claude"},
		harnessSets: &[]server.SetDefaultHarnessRequest{},
		harnessRes:  server.SetDefaultHarnessResult{Refused: true, Message: "codex is not on PATH"},
	}, "")
	m, _ = runKey(t, m, tea.KeyPressMsg{Code: 'H', Text: "H"})
	m, _ = runKey(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	_, toast := next.(QueueModel).Update(cmd())
	if toast == nil {
		t.Fatal("no toast for the refusal")
	}
	got, ok := toast().(notify.NotifyMsg)
	if !ok || !strings.Contains(got.Message, "refused: codex is not on PATH") {
		t.Fatalf("toast = %#v", toast())
	}
}

func TestDefaultHarness_NoServerWarnsAndOpensNothing(t *testing.T) {
	m := NewQueueModel(t.TempDir(), ui.Settings{}, keys.New(nil))
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'H', Text: "H"})
	if next.(QueueModel).harnessPicker.open {
		t.Fatal("picker opened without a server")
	}
	got, ok := cmd().(notify.NotifyMsg)
	if !ok || got.Message != serverNotRunning {
		t.Fatalf("toast = %#v", cmd())
	}
}

func TestDefaultHarness_HelpListsTheBinding(t *testing.T) {
	m := harnessQueue(t, fakeServerAPI{}, "")
	m.help.Open(m.width, m.height)
	if view := m.help.View(); !strings.Contains(view, "Default harness...") {
		t.Fatalf("help lacks the binding:\n%s", view)
	}
}
