package tickets

import (
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
)

func finishedQueue(t *testing.T, now time.Time) QueueModel {
	t.Helper()
	snap := server.Snapshot{Seq: 1, RecentlyFinished: []server.FinishedEpic{
		{Name: "widgets", Project: "gx", Done: 4, Total: 5, CompletedAt: now.Add(-2 * time.Hour)},
	}}
	m, _ := deliverQueue(t, snap, nil)
	m.now = func() time.Time { return now }
	return m
}

func TestQueueFinished_RendersDimLine(t *testing.T) {
	m := finishedQueue(t, time.Now())

	body := strings.Join(m.queueBody(100), "\n")
	if want := "widgets gx 4/5 finished 2h ago"; !strings.Contains(body, want) {
		t.Errorf("queue body = %q, want it to contain %q", body, want)
	}
}

func TestQueueFinished_TickRerendersRelativeTime(t *testing.T) {
	now := time.Now()
	m := finishedQueue(t, now)
	next, _ := m.Update(queueSpinnerRestartMsg{})
	m = next.(QueueModel)

	m.now = func() time.Time { return now.Add(time.Hour) }
	next, cmd := m.Update(finishedTickMsg{gen: m.finishedGen})
	m = next.(QueueModel)

	if cmd == nil {
		t.Error("tick did not schedule the next one")
	}
	body := strings.Join(m.queueBody(100), "\n")
	if want := "finished 3h ago"; !strings.Contains(body, want) {
		t.Errorf("queue body = %q, want it to contain %q", body, want)
	}
}

func TestQueueFinished_NoTickChainWhileHidden(t *testing.T) {
	m := finishedQueue(t, time.Now())

	next, cmd := m.Update(queueSpinnerRestartMsg{})
	if cmd == nil {
		t.Fatal("activation did not start the refresh tick")
	}
	m = next.(QueueModel)

	// A tick from the chain before the latest activation ends the old chain.
	if _, cmd := m.Update(finishedTickMsg{gen: m.finishedGen - 1}); cmd != nil {
		t.Error("stale tick re-armed itself")
	}
}
