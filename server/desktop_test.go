package server

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/notifykind"
	"github.com/elentok/gx/osnotify"
	"github.com/elentok/gx/ralphloop/notify"
	"github.com/elentok/gx/server/settings"
	"github.com/elentok/gx/sound"
	"github.com/elentok/gx/subscription"
	"github.com/elentok/gx/testutil/runnerfake"
)

type bannerLog struct {
	mu   sync.Mutex
	sent []osnotify.Notification
}

func (l *bannerLog) Notify(_ context.Context, n osnotify.Notification) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sent = append(l.sent, n)
	return nil
}

func (l *bannerLog) all() []osnotify.Notification {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]osnotify.Notification(nil), l.sent...)
}

// desktopServer is a server whose banners and sounds are recorded and whose
// Desktop channel is ch.
func desktopServer(t *testing.T, ch settings.NotifyChannel) (*Server, *bannerLog, *recPlayer) {
	t.Helper()
	stateDir, err := os.MkdirTemp("", "gx") // short path: unix socket limit
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(stateDir) })
	banners, player := &bannerLog{}, &recPlayer{}
	sounds := config.AgentsSoundsConfig{Enabled: true}
	s, err := New(Config{Runner: runnerfake.NewRunner(), StateDir: stateDir, TicketStore: t.TempDir(), Notifier: banners, Player: player,
		Settings:        Settings{BudgetSoftLimit: 10, BudgetHardLimit: 20, AgentsBanners: true, AgentsSounds: sounds, NotifyChannels: settings.NotifyChannels{Desktop: ch}},
		ExtraUsageCheck: func() subscription.State { return subscription.StateDisabled }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.chat.Close() })
	return s, banners, player
}

func overSoftLimit(s *Server) {
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.Local)
	s.budget.SeedNotes(now)
	s.budget.RecordSpendAt("a", 11, now)
	s.budget.NotifyAt(now)
}

func TestDesktop_ACheckedNonSessionKindIsABannerWithNoSound(t *testing.T) {
	t.Parallel()
	s, banners, player := desktopServer(t, settings.NotifyChannel{Kinds: []string{notifykind.BudgetAndUsage}})
	overSoftLimit(s)
	got := banners.all()
	if len(got) != 1 || !strings.Contains(got[0].Title, "Budget soft limit reached") {
		t.Fatalf("banners = %+v, want one with the chat headline as title", got)
	}
	if player.n.Load() != 0 {
		t.Errorf("sounds = %d, want none for a non-session kind", player.n.Load())
	}
}

func TestDesktop_AnUncheckedKindIsNoBannerAndNoSound(t *testing.T) {
	t.Parallel()
	s, banners, player := desktopServer(t, settings.NotifyChannel{Kinds: []string{notifykind.SessionAlert}})
	overSoftLimit(s)
	if got := banners.all(); len(got) != 0 || player.n.Load() != 0 {
		t.Errorf("banners = %+v, sounds = %d, want none", got, player.n.Load())
	}
}

func TestDesktop_SwitchOffSendsNothing(t *testing.T) {
	t.Parallel()
	s, banners, player := desktopServer(t, settings.NotifyChannel{Off: true})
	overSoftLimit(s)
	(&desktopAdapter{s: s, webURL: func() string { return "" }}).Show(notify.DesktopNote{Kind: notifykind.SessionAlert, Session: true, Sound: sound.Done})
	// The decorators drop the system banners and sounds that bypass the adapter.
	_ = s.cfg.Notifier.Notify(context.Background(), osnotify.Notification{Title: "storm"})
	s.cfg.Player.Play(context.Background(), config.AgentsSoundsConfig{Enabled: true}, sound.Done)
	if got := banners.all(); len(got) != 0 || player.n.Load() != 0 {
		t.Errorf("banners = %+v, sounds = %d, want none", got, player.n.Load())
	}
}

func TestDesktop_OnlySessionKindsPlayASoundAndEveryTargetCarriesItsLink(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		note     notify.DesktopNote
		wantLink string
		sounds   int
	}{
		{"ticket", notify.DesktopNote{Kind: notifykind.PausedTicket, Target: notify.Target{Ticket: "p:e/01"}}, "http://web/#/t/p:e/01", 0},
		{"epic", notify.DesktopNote{Kind: notifykind.PausedTicket, Target: notify.Target{Epic: "p:e"}}, "http://web/#/e/p:e", 0},
		{"design", notify.DesktopNote{Kind: notifykind.PausedTicket, Target: notify.Target{Design: "p:m"}}, "http://web/#/d/p:m", 0},
		{"session", notify.DesktopNote{Kind: notifykind.PausedTicket, Target: notify.Target{Session: "abc"}}, "http://web/#/agents/abc", 0},
		{"none", notify.DesktopNote{Kind: notifykind.Disk}, "http://web/#/", 0},
		{"session banner keeps its own link", notify.DesktopNote{Kind: notifykind.SessionAlert, Session: true, Link: "L", Sound: sound.Done}, "L", 1},
		{"non-session kind never plays", notify.DesktopNote{Kind: notifykind.Disk, Sound: sound.Done}, "http://web/#/", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, banners, player := desktopServer(t, settings.NotifyChannel{})
			a := &desktopAdapter{s: s, webURL: func() string { return "http://web" }}
			a.Show(tc.note)
			got := banners.all()
			if len(got) != 1 || got[0].Link != tc.wantLink {
				t.Errorf("banners = %+v, want one with Link %q", got, tc.wantLink)
			}
			if int(player.n.Load()) != tc.sounds {
				t.Errorf("sounds = %d, want %d", player.n.Load(), tc.sounds)
			}
		})
	}
}

func TestDesktop_NoWebListenerMeansNoLink(t *testing.T) {
	t.Parallel()
	s, banners, _ := desktopServer(t, settings.NotifyChannel{})
	(&desktopAdapter{s: s, webURL: func() string { return "" }}).Show(notify.DesktopNote{Kind: notifykind.Disk, Target: notify.Target{Epic: "p:e"}})
	if got := banners.all(); len(got) != 1 || got[0].Link != "" {
		t.Errorf("banners = %+v, want one with no Link", got)
	}
}
