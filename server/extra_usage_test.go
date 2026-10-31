package server

import (
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/subscription"
)

func TestExtraUsage_EnqueueWarnsAndNotifiesOncePerDay(t *testing.T) {
	t.Parallel()
	s, wait := chatServerFlushing(t, 0, 0, 100*time.Millisecond)
	s.cfg.ExtraUsageCheck = func() subscription.State { return subscription.StateEnabled }

	for i := 0; i < 2; i++ {
		res, err := s.withExtraUsage("gx", []string{"claude"}, QueueResult{}, nil)
		if err != nil || res.Refused {
			t.Fatalf("replace = %+v, %v", res, err)
		}
		if !res.ExtraUsage {
			t.Fatalf("replace %d: ExtraUsage = false, want the warning", i)
		}
	}
	got := wait(1)
	if len(got) != 1 || !strings.Contains(got[0], "extra usage") {
		t.Fatalf("sends = %v, want one extra-usage notice", got)
	}

	// A new day announces again.
	s.checkExtraUsage(time.Now().AddDate(0, 0, 1))
	if got := wait(2); len(got) != 2 {
		t.Fatalf("sends = %v, want a second notice the next day", got)
	}
}

func TestExtraUsage_OffOrSuppressedStaysQuiet(t *testing.T) {
	t.Parallel()
	s, wait := chatServer(t, 0, 0)
	s.cfg.ExtraUsageCheck = func() subscription.State { return subscription.StateDisabled }
	if s.checkExtraUsage(time.Now()) {
		t.Fatal("disabled: warned")
	}
	s.cfg.ExtraUsageCheck = func() subscription.State { return subscription.StateEnabled }
	s.settings.Store(&Settings{SuppressExtraUsageWarning: true})
	if s.checkExtraUsage(time.Now()) {
		t.Fatal("suppressed: warned")
	}
	s.chat.Close()
	if got := wait(-1); len(got) != 0 {
		t.Fatalf("sends = %v, want none", got)
	}
}

func TestExtraUsage_OnlyClaudeItemsWarn(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		def     string
		agents  []string
		warning bool
	}{
		{"claude item", "", []string{"claude"}, true},
		{"codex item", "", []string{"codex"}, false},
		{"follows a Claude default", "claude", []string{""}, true},
		{"follows an unset default", "", []string{""}, true},
		{"follows a Codex default", "codex", []string{""}, false},
		{"explicit claude beats a Codex default", "codex", []string{"claude"}, true},
		{"replace with Claude and Codex", "codex", []string{"codex", "claude"}, true},
		{"replace with only Codex", "claude", []string{"codex", "codex"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, wait := chatServer(t, 0, 0)
			s.cfg.ExtraUsageCheck = func() subscription.State { return subscription.StateEnabled }
			s.settings.Store(&Settings{AgentsDefaultHarness: tc.def})
			res, err := s.withExtraUsage("gx", tc.agents, QueueResult{}, nil)
			if err != nil || res.ExtraUsage != tc.warning {
				t.Fatalf("ExtraUsage = %v, %v; want %v", res.ExtraUsage, err, tc.warning)
			}
			s.chat.Close()
			if got := wait(-1); (len(got) == 1) != tc.warning {
				t.Fatalf("sends = %v, want a notice: %v", got, tc.warning)
			}
		})
	}
}

func TestExtraUsage_StartCheckQuietWhenDefaultIsCodex(t *testing.T) {
	t.Parallel()
	for def, notices := range map[string]int{"codex": 0, "claude": 1, "": 1} {
		t.Run("default="+def, func(t *testing.T) {
			t.Parallel()
			s, wait := chatServer(t, 0, 0)
			s.cfg.ExtraUsageCheck = func() subscription.State { return subscription.StateEnabled }
			s.settings.Store(&Settings{AgentsDefaultHarness: def})
			s.startExtraUsageCheck(time.Now())
			s.chat.Close()
			if got := wait(-1); len(got) != notices {
				t.Fatalf("sends = %v, want %d", got, notices)
			}
		})
	}
}
