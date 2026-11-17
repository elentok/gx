package queue

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/elentok/gx/agentlog"
	"github.com/elentok/gx/config"
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

func fakeAgentLog(t *testing.T, f func(context.Context, config.Dirs, string) (agentlog.Log, error)) {
	t.Helper()
	prev := rowactions.AgentLog
	t.Cleanup(func() { rowactions.AgentLog = prev })
	rowactions.AgentLog = f
}

func fakeNativeLog(t *testing.T, log agentlog.Log, err error) {
	t.Helper()
	fakeAgentLog(t, func(context.Context, config.Dirs, string) (agentlog.Log, error) {
		return log, err
	})
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
