package tickets

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ui/notify"
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
		"enqueue": serverEnqueuedMsg{added: 1},
		"replace": serverReplacedMsg{count: 1},
		"action":  serverDoneMsg{ok: "parked"},
	} {
		_, cmd := m.Update(msg)
		assertOnlyToast(t, name, cmd)
	}
}

// The status menu and the answer editor both finish with statusChangedMsg.
func TestStatusChanged_UpModeMakesNoRefetch(t *testing.T) {
	_, cmd := newServerModel(t).Update(statusChangedMsg{})
	if cmd != nil {
		t.Errorf("up mode returned a command (%T); the stream event carries the change", cmd())
	}
}

func TestStatusChanged_DownModeReloadsDisk(t *testing.T) {
	down, _ := withLink(newServerModel(t), ServerLinkDown)
	_, cmd := down.Update(statusChangedMsg{})
	if cmd == nil {
		t.Fatal("down mode did not reload")
	}
	if _, ok := cmd().(epicsLoadedMsg); !ok {
		t.Errorf("down mode reload = %T, want epicsLoadedMsg", cmd())
	}
}

func TestWrites_QueueTabOnlyToasts(t *testing.T) {
	m, _ := loadedServerQueue(t, "open")
	for name, msg := range map[string]tea.Msg{
		"remove": serverRemovedMsg{removed: 1},
		"action": serverDoneMsg{ok: "queue paused"},
	} {
		_, cmd := m.Update(msg)
		assertOnlyToast(t, name, cmd)
	}
}
