package budget

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop/notify"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/subscription"
)

// chatBudget is a Budget wired to a fake Slack webhook that records each body.
// sent waits for n bodies, then returns every body seen.
func chatBudget(t *testing.T, state subscription.State, set settings.Settings) (*Budget, func(n int) []string) {
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
	chat := notify.NewServerChat(notify.ServerChatConfig{
		SlackWebhookURL: hook.URL, GateStatePath: filepath.Join(t.TempDir(), "gate.json"), FlushInterval: 50 * time.Millisecond,
	})
	t.Cleanup(chat.Close)
	var h settings.Holder
	h.Store(&set)
	b := &Budget{
		cfg:      Config{ExtraUsageCheck: func() subscription.State { return state }},
		settings: &h,
		chat:     chat,
	}
	sent := func(n int) []string {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			mu.Lock()
			got := len(bodies)
			mu.Unlock()
			if got >= n {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		time.Sleep(300 * time.Millisecond) // anything extra would show up now
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), bodies...)
	}
	return b, sent
}

func TestExtraUsage_WarnsAndNotifiesOncePerDay(t *testing.T) {
	t.Parallel()
	b, sent := chatBudget(t, subscription.StateEnabled, settings.Settings{})
	now := time.Now()
	for i := 0; i < 2; i++ {
		if !b.CheckExtraUsage(now) {
			t.Fatalf("check %d: not warned, want the warning", i)
		}
	}
	got := sent(1)
	if len(got) != 1 || !strings.Contains(got[0], "extra usage") {
		t.Fatalf("sends = %v, want one extra-usage notice", got)
	}

	// A new day announces again.
	b.CheckExtraUsage(now.AddDate(0, 0, 1))
	if got := sent(2); len(got) != 2 {
		t.Fatalf("sends = %v, want a second notice the next day", got)
	}
}

func TestExtraUsage_OffOrSuppressedStaysQuiet(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		state subscription.State
		set   settings.Settings
	}{
		"disabled":   {subscription.StateDisabled, settings.Settings{}},
		"suppressed": {subscription.StateEnabled, settings.Settings{SuppressExtraUsageWarning: true}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			b, sent := chatBudget(t, tc.state, tc.set)
			if b.CheckExtraUsage(time.Now()) {
				t.Fatal("warned")
			}
			if got := sent(-1); len(got) != 0 {
				t.Fatalf("sends = %v, want none", got)
			}
		})
	}
}

func TestExtraUsage_StartCheckQuietWhenDefaultIsCodex(t *testing.T) {
	t.Parallel()
	for def, notices := range map[string]int{"codex": 0, "claude": 1, "": 1} {
		t.Run("default="+def, func(t *testing.T) {
			t.Parallel()
			b, sent := chatBudget(t, subscription.StateEnabled, settings.Settings{AgentsDefaultHarness: def})
			b.StartExtraUsageCheck(time.Now())
			if got := sent(notices); len(got) != notices {
				t.Fatalf("sends = %v, want %d", got, notices)
			}
		})
	}
}
