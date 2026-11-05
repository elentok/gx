package tickets

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/tickets/queue"
	"github.com/elentok/gx/ui/tickets/rowactions"
	"github.com/elentok/gx/ui/tickets/tabapi"
)

// A finished write only toasts: the new state arrives on the event stream, so
// the tab must not fetch the queue or a snapshot itself.
func assertOnlyToast(t *testing.T, name string, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatalf("%s: no toast", name)
	}
	if _, ok := cmd().(notify.NotifyMsg); !ok {
		t.Errorf("%s: command is not a bare toast (a batch means a refetch rode along)", name)
	}
}

func TestWrites_TicketsTabOnlyToasts(t *testing.T) {
	m := newServerModel(t)
	for name, msg := range map[string]tea.Msg{
		"epic":   rowactions.EpicQueuedMsg{Refusals: []string{"gx:a/01: refused"}},
		"action": queue.ServerDoneMsg{OK: "paused"},
	} {
		_, cmd := m.Update(msg)
		assertOnlyToast(t, name, cmd)
	}
}

// The status menu and the answer editor both finish with StatusChangedMsg.
func TestStatusChanged_UpModeMakesNoRefetch(t *testing.T) {
	_, cmd := newServerModel(t).Update(rowactions.StatusChangedMsg{})
	if cmd != nil {
		t.Errorf("up mode returned a command (%T); the stream event carries the change", cmd())
	}
}

func TestStatusChanged_DownModeReloadsDisk(t *testing.T) {
	down, _ := withLink(newServerModel(t), tabapi.ServerLinkDown)
	_, cmd := down.Update(rowactions.StatusChangedMsg{})
	if cmd == nil {
		t.Fatal("down mode did not reload")
	}
	if _, ok := cmd().(epicsLoadedMsg); !ok {
		t.Errorf("down mode reload = %T, want epicsLoadedMsg", cmd())
	}
}

// Seam: a successful add (single or epic) toasts and switches to the Queue
// tab; a total failure only toasts.
func TestWrites_AddToQueueSwitchesToQueueTab(t *testing.T) {
	m := newServerModel(t)
	for name, msg := range map[string]tea.Msg{
		"epic": rowactions.EpicQueuedMsg{Queued: 2},
	} {
		_, cmd := m.Update(msg)
		if cmd == nil {
			t.Fatalf("%s: no command", name)
		}
		var switched bool
		for _, c := range cmd().(tea.BatchMsg) {
			if vs, ok := nav.IsSwitch(c()); ok && vs.Tab == nav.TabQueue {
				switched = true
			}
		}
		if !switched {
			t.Errorf("%s: did not switch to the Queue tab", name)
		}
	}
}
