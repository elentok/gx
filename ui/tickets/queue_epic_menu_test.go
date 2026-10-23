package tickets

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// queueOnEpicHeader is a server-mode Queue tab with gx:alpha's header
// selected.
func queueOnEpicHeader(t *testing.T, api fakeServerAPI) QueueModel {
	t.Helper()
	m := newMaintenanceQueue(t, api)
	m.epics[0].Path = "gx:alpha"
	m.clampSelected()
	m.selectFirstRow()
	if _, ok := m.selectedQueueEpic(); !ok {
		t.Fatal("first selectable row is not the epic header")
	}
	return m
}

func pressQueueMenuKey(t *testing.T, m QueueModel) QueueModel {
	t.Helper()
	next, _ := m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	m = next.(QueueModel)
	if !m.ticketMenu.IsOpen {
		t.Fatal("m on the epic header opened no menu")
	}
	return m
}

func TestQueueEpicMenu_HeaderOpensTheFullEpicMenu(t *testing.T) {
	m := pressQueueMenuKey(t, queueOnEpicHeader(t, fakeServerAPI{calls: &[]string{}}))

	view := m.ticketMenu.View()
	items := epicMenuItems(epicMenuInput{epic: m.epics[0]})
	if len(items) == 0 {
		t.Fatal("epic menu has no items")
	}
	for _, it := range items {
		if !strings.Contains(view, it.Label) {
			t.Errorf("menu lacks %q:\n%s", it.Label, view)
		}
	}
	for _, it := range items {
		if it.Label == "Check all" && it.Disabled != "" {
			t.Errorf("Check all is dim with %q", it.Disabled)
		}
	}
}

func TestQueueEpicMenu_MergeByAddress(t *testing.T) {
	var calls []string
	m := queueOnEpicHeader(t, fakeServerAPI{calls: &calls})
	m.epics[0].Tickets[0].Status = "done"
	m.clampSelected()

	_, cmd := chooseQueueMenuItem(t, pressQueueMenuKey(t, m), "Merge")
	if cmd == nil {
		t.Fatal("Merge issued no cmd")
	}
	cmd()
	if !slices.Equal(calls, []string{"merge gx:alpha"}) {
		t.Errorf("calls = %v, want [merge gx:alpha]", calls)
	}
}

func TestQueueEpicMenu_SetStatusWritesTheEpicFileAndPingsTheServer(t *testing.T) {
	store := t.TempDir()
	epicMD := filepath.Join(store, "gx", "alpha", "ticket.md")
	if err := os.MkdirAll(filepath.Dir(epicMD), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(epicMD, []byte("---\nstatus: draft\n---\n\n# alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var calls []string
	m := queueOnEpicHeader(t, fakeServerAPI{calls: &calls})
	m.settings.TicketStore = store

	_, cmd := chooseQueueMenuItem(t, pressQueueMenuKey(t, m), "Set open")
	if cmd == nil {
		t.Fatal("Set open issued no cmd")
	}
	if msg := cmd(); msg != (statusChangedMsg{}) {
		t.Fatalf("Set open = %#v", msg)
	}
	if raw, _ := os.ReadFile(epicMD); !strings.Contains(string(raw), "status: open") {
		t.Errorf("ticket.md = %s, want status: open", raw)
	}
	if !slices.Equal(calls, []string{"changed gx:alpha"}) {
		t.Errorf("calls = %v, want [changed gx:alpha]", calls)
	}
}

func TestQueueEpicMenu_CheckedSetKeysIgnoreTheHeader(t *testing.T) {
	var calls []string
	m := queueOnEpicHeader(t, fakeServerAPI{calls: &calls})
	m.epics[0].Tickets[0].Body = "## Proposed Remedy\n\nrun it"
	for _, key := range []rune{'x', 'A', ' '} {
		if got := pressQueue(t, m, key); got.confirm.IsOpen {
			t.Errorf("%q on the header opened a confirm", key)
		}
	}
	if len(calls) != 0 {
		t.Errorf("calls = %v, want none", calls)
	}
}

func TestQueueEpicMenu_EditTargetIsTheEpicFile(t *testing.T) {
	store := t.TempDir()
	epicMD := filepath.Join(store, "gx", "alpha", "ticket.md")
	if err := os.MkdirAll(filepath.Dir(epicMD), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(epicMD, []byte("---\nstatus: draft\n---\n\n# alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := queueOnEpicHeader(t, fakeServerAPI{})
	m.settings.TicketStore = store

	if got, ok := m.selectedEditTarget(); !ok || got != epicMD {
		t.Errorf("edit target = %q, %v, want %q", got, ok, epicMD)
	}
}

func TestQueueEpicMenu_SelectedHeaderRendersTheSelectionMark(t *testing.T) {
	m := queueOnEpicHeader(t, fakeServerAPI{})
	for _, line := range m.queueBody(80) {
		plain := ansi.Strip(line)
		if strings.Contains(plain, "gx:alpha") {
			if !strings.HasPrefix(plain, "▌") {
				t.Errorf("header line %q lacks the selection mark", plain)
			}
			return
		}
	}
	t.Fatal("no header line rendered")
}

func TestQueueEpicMenu_SStillStartsTheServer(t *testing.T) {
	m := newServerQueueModel(t, func(context.Context) error { return nil })
	if _, ok := m.selectedQueueEpic(); !ok {
		t.Fatal("header not selected")
	}
	next, _ := m.WithServerState(nil, ServerLinkDown)
	next, _ = next.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	if !next.(QueueModel).confirm.IsOpen {
		t.Error("s on the header did not offer to start the server")
	}
}
