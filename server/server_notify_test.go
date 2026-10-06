package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop"
)

// chatServer is a server wired to a fake Slack webhook that records each body.
func chatServer(t *testing.T, soft, hard float64) (*Server, func(n int) []string) {
	t.Helper()
	var mu sync.Mutex
	var bodies []string
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		mu.Unlock()
	}))
	t.Cleanup(hook.Close)
	stateDir, err := os.MkdirTemp("", "gx") // short path: unix socket limit
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(stateDir) })
	s, err := New(Config{StateDir: stateDir, TicketStore: t.TempDir(), BudgetSoftLimit: soft, BudgetHardLimit: hard,
		Chat: ralphloop.ServerChatConfig{SlackWebhookURL: hook.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json")}})
	if err != nil {
		t.Fatal(err)
	}
	// wait returns what was sent once n messages arrived, or after a grace
	// period when fewer ever will; n < 0 just waits out the grace period.
	wait := func(n int) []string {
		deadline := time.Now().Add(10 * time.Second)
		if n < 0 {
			deadline = time.Now().Add(1500 * time.Millisecond)
		}
		for time.Now().Before(deadline) {
			mu.Lock()
			got := len(bodies)
			mu.Unlock()
			if n >= 0 && got >= n {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		time.Sleep(500 * time.Millisecond) // anything extra would show up now
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), bodies...)
	}
	return s, wait
}

func TestServerNotices_RestartSendsOneServerLevelMessage(t *testing.T) {
	s, wait := chatServer(t, 0, 0)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx) }()

	got := wait(1)
	cancel()
	<-done
	if len(got) != 1 || !strings.Contains(got[0], "server started") {
		t.Fatalf("sends = %v, want one 'server started'", got)
	}
}

func TestServerNotices_BudgetLimitLatchesOncePerDay(t *testing.T) {
	s, wait := chatServer(t, 10, 20)
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.Local)
	s.seedBudgetNotes(now)

	s.ledger.record("a", 11, now)
	s.notifyBudget(now)
	s.ledger.record("a", 12, now.Add(time.Minute))
	s.notifyBudget(now.Add(time.Minute)) // still over the soft limit: no repeat
	s.ledger.record("a", 21, now.Add(2*time.Minute))
	s.notifyBudget(now.Add(2 * time.Minute))

	s.chat.Close()
	got := wait(-1)
	if len(got) != 1 {
		t.Fatalf("sends = %d, want the soft and hard notices batched in 1: %v", len(got), got)
	}
	if !strings.Contains(got[0], "soft limit") || !strings.Contains(got[0], "hard limit") {
		t.Errorf("message lacks a limit notice: %s", got[0])
	}
}

func TestServerNotices_BudgetAlreadyOverAtStartIsNotAnnounced(t *testing.T) {
	s, wait := chatServer(t, 10, 0)
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.Local)
	s.ledger.record("a", 11, now)
	s.seedBudgetNotes(now)
	s.notifyBudget(now)

	s.chat.Close()
	if got := wait(-1); len(got) != 0 {
		t.Fatalf("sends = %v, want none", got)
	}
}

func TestServerNotices_DayRolloverSendsTheSummary(t *testing.T) {
	s, wait := chatServer(t, 0, 0)
	now := time.Date(2026, 10, 6, 23, 59, 0, 0, time.Local)
	s.seedBudgetNotes(now)
	s.ledger.record("a", 4, now)
	s.notifyBudget(now) // same day: nothing

	s.notifyBudget(now.Add(2 * time.Minute))
	s.chat.Close()
	got := wait(-1)
	if len(got) != 1 || !strings.Contains(got[0], "daily summary") || !strings.Contains(got[0], "2026-10-06") {
		t.Fatalf("sends = %v, want one daily summary for 2026-10-06", got)
	}
}
