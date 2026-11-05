package tickets

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/agentlog"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/ui/tickets/rowactions"
)

func fakeNativeLog(t *testing.T, log agentlog.Log, err error) {
	t.Helper()
	prev := rowactions.AgentLog
	t.Cleanup(func() { rowactions.AgentLog = prev })
	rowactions.AgentLog = func(context.Context, config.Dirs, string, rowactions.HerdrLookup) (agentlog.Log, error) {
		return log, err
	}
}

// requestMenu selects the first of two tickets and presses "m", returning the
// model and the not-yet-delivered ready message.
func requestMenu(t *testing.T) (Model, tea.Msg) {
	t.Helper()
	fakeNativeLog(t, agentlog.Log{}, agentlog.ErrNoAgent)
	m := withState(newServerModel(t), wire.Snapshot{Tickets: []wire.TicketInfo{
		{Address: "gx:alpha/01", Title: "First", Status: "claimed"},
		{Address: "gx:alpha/02", Title: "Second", Status: "claimed"},
	}})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = selectTicketRow(t, updated.(Model))
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	return updated.(Model), cmd()
}

func TestMenuReady_DroppedWhenStale(t *testing.T) {
	cases := map[string]tea.KeyPressMsg{
		"esc":              {Code: tea.KeyEscape},
		"selection change": {Code: 'j', Text: "j"},
		"help open":        {Code: '?', Text: "?"},
	}
	for name, key := range cases {
		t.Run(name, func(t *testing.T) {
			m, ready := requestMenu(t)
			updated, _ := m.Update(key)
			updated, _ = updated.(Model).Update(ready)
			if updated.(Model).menus.Menu.IsOpen {
				t.Error("stale ready message opened the menu")
			}
		})
	}
}

func TestMenuReady_DroppedAfterLeavingAndReturning(t *testing.T) {
	m, ready := requestMenu(t)
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	updated, _ = updated.(Model).Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	updated, _ = updated.(Model).Update(ready)
	if updated.(Model).menus.Menu.IsOpen {
		t.Error("ready message opened the menu after the selection moved away")
	}
}

func TestMenuReady_DeliveredOnce(t *testing.T) {
	m, ready := requestMenu(t)
	updated, _ := m.Update(ready)
	updated, _ = updated.(Model).Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	updated, _ = updated.(Model).Update(ready)
	if updated.(Model).menus.Menu.IsOpen {
		t.Error("a second delivery of the same ready message reopened the menu")
	}
}
