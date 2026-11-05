package queue

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/agentlog"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/contextmenu"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tickets/rowactions"
)

const watchLogLine = `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hello from the agent"}]}}` + "\n"

func appendWatchLog(t *testing.T, path string, texts ...string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, text := range texts {
		if _, err := f.WriteString(strings.Replace(watchLogLine, "hello from the agent", text, 1)); err != nil {
			t.Fatal(err)
		}
	}
}

func fakeAgentLog(t *testing.T, f func(context.Context, config.Dirs, string, rowactions.HerdrLookup) (agentlog.Log, error)) {
	t.Helper()
	prev := rowactions.AgentLog
	t.Cleanup(func() { rowactions.AgentLog = prev })
	rowactions.AgentLog = f
}

func fakeNativeLog(t *testing.T, log agentlog.Log, err error) {
	t.Helper()
	fakeAgentLog(t, func(context.Context, config.Dirs, string, rowactions.HerdrLookup) (agentlog.Log, error) {
		return log, err
	})
}

// fakeHerdrOnlyLog runs the real locator against an empty state dir, so every
// log comes from the herdr lookup the tab hands it.
func fakeHerdrOnlyLog(t *testing.T) {
	t.Helper()
	stateDir := t.TempDir()
	fakeAgentLog(t, func(ctx context.Context, _ config.Dirs, address string, herdr rowactions.HerdrLookup) (agentlog.Log, error) {
		a, err := tickets.ParseAddress(address, tickets.AddressContext{})
		if err != nil {
			return agentlog.Log{}, err
		}
		return agentlog.Locator{StateDir: stateDir, Herdr: herdr}.AgentLog(ctx, a)
	})
}

// herdrAPI is a server running one herdr iteration of agent on gx:alpha/01.
func herdrAPI(agent, transcript string) tickettest.FakeServerAPI {
	return tickettest.FakeServerAPI{IterationsRes: []wire.IterationInfo{
		{Address: "gx:alpha/01", Agent: agent, Transcript: transcript, Pane: "p1", Tab: "t1"},
	}}
}

// fakeFocus records the herdr tabs focused.
func fakeFocus(t *testing.T) *[]string {
	t.Helper()
	var focused []string
	prev := rowactions.FocusIterationTab
	t.Cleanup(func() { rowactions.FocusIterationTab = prev })
	rowactions.FocusIterationTab = func(tab string) error { focused = append(focused, tab); return nil }
	return &focused
}

func hasItem(items []contextmenu.Item[rowactions.TicketTarget], label string) bool {
	return slices.ContainsFunc(items, func(it contextmenu.Item[rowactions.TicketTarget]) bool { return it.Label == label })
}

func TestWatch_NoOrPrunedLogDims(t *testing.T) {
	cases := map[string]struct {
		log agentlog.Log
		err error
	}{
		"no agent": {agentlog.Log{}, agentlog.ErrNoAgent},
		"pruned":   {agentlog.Log{Pruned: true}, nil},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			fakeNativeLog(t, c.log, c.err)
			m := selectFirstQueueTicketRow(t, newServerQueueModel(t, nil))
			_, ready := requestQueueMenu(t, m)
			if got := itemByLabel(t, ready.Items, "Watch").Disabled; got != rowactions.NoAgentLog {
				t.Errorf("Watch disabled = %q, want %q", got, rowactions.NoAgentLog)
			}
		})
	}
}

func TestGoToPane_HerdrOnlyAndFocusesPane(t *testing.T) {
	fakeHerdrOnlyLog(t)
	focused := fakeFocus(t)
	m := selectFirstQueueTicketRow(t, newServerQueueModel(t, nil))
	if _, ready := requestQueueMenu(t, m); hasItem(ready.Items, "Go to pane") {
		t.Error("Go to pane shown for a ticket with no herdr agent")
	}
	m.server.ServerAPI, m.srv.Reads = herdrAPI("codex", ""), tickettest.FakeReads{FakeServerAPI: herdrAPI("codex", "")}
	_, cmd := chooseQueueMenuItem(t, openQueueMenu(t, m), "Go to pane")
	if cmd == nil {
		t.Fatal("Go to pane returned no Cmd")
	}
	cmd()
	if !slices.Equal(*focused, []string{"t1"}) {
		t.Errorf("focused = %v, want [t1]", *focused)
	}
}
