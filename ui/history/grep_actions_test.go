package history

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/claudehistory"
	"github.com/elentok/gx/ui/notify"
)

func TestCmdYankGrepSessionID_NoResults_Warns(t *testing.T) {
	m := newTestModel()
	m, _ = update(m, tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})

	cmd := m.cmdYankGrepSessionID()
	if cmd == nil {
		t.Fatal("expected a warning cmd for empty grep results")
	}
	if msg, ok := cmd().(notify.NotifyMsg); !ok || msg.Kind != notify.KindWarning {
		t.Fatalf("expected a warning NotifyMsg, got %#v", cmd())
	}
}

func TestCmdYankGrepSessionID_WritesToClipboard(t *testing.T) {
	prev := historyClipboardWrite
	var written string
	historyClipboardWrite = func(s string) error {
		written = s
		return nil
	}
	t.Cleanup(func() { historyClipboardWrite = prev })

	m := newTestModel()
	m, _ = update(m, tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	m.grepResults = []claudehistory.GrepResult{{SessionID: "sess-1"}}
	m.grepList.SetSelected(0, 1)

	cmd := m.cmdYankGrepSessionID()
	if cmd == nil {
		t.Fatal("expected a non-nil cmd")
	}
	cmd()
	if written != "sess-1" {
		t.Fatalf("clipboard write = %q, want sess-1", written)
	}
}

func TestCmdYankGrepSessionID_ClipboardError_ReportsError(t *testing.T) {
	prev := historyClipboardWrite
	historyClipboardWrite = func(string) error { return errors.New("no clipboard") }
	t.Cleanup(func() { historyClipboardWrite = prev })

	m := newTestModel()
	m, _ = update(m, tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	m.grepResults = []claudehistory.GrepResult{{SessionID: "sess-1"}}
	m.grepList.SetSelected(0, 1)

	cmd := m.cmdYankGrepSessionID()
	if cmd == nil {
		t.Fatal("expected a non-nil cmd")
	}
	if msg, ok := cmd().(notify.NotifyMsg); !ok || msg.Kind != notify.KindError {
		t.Fatalf("expected an error NotifyMsg, got %#v", cmd())
	}
}

func TestCtrlY_OnGrepPage_YanksSessionID(t *testing.T) {
	prev := historyClipboardWrite
	var written string
	historyClipboardWrite = func(s string) error {
		written = s
		return nil
	}
	t.Cleanup(func() { historyClipboardWrite = prev })

	m := newTestModel()
	m, _ = update(m, tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	m.grepResults = []claudehistory.GrepResult{{SessionID: "sess-1"}}
	m.grepList.SetSelected(0, 1)

	m, cmd := update(m, tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("expected ctrl+y to produce a cmd")
	}
	cmd()
	if written != "sess-1" {
		t.Fatalf("clipboard write = %q, want sess-1", written)
	}
}
