package server

import (
	"context"
	"sync"
	"time"

	"github.com/elentok/gx/osnotify"
	"github.com/elentok/gx/sound"
)

const defaultSessionBannerDelay = time.Second

// termBundleIDs maps a TUI's TERM_PROGRAM to the app a banner click activates.
var termBundleIDs = map[string]string{
	"Apple_Terminal": "com.apple.Terminal",
	"iTerm.app":      "com.googlecode.iterm2",
	"ghostty":        "com.mitchellh.ghostty",
	"WezTerm":        "com.github.wez.wezterm",
	"vscode":         "com.microsoft.VSCode",
}

// sessionBanners holds the trigger each session's pending banner merges into.
type sessionBanners struct {
	mu      sync.Mutex
	pending map[string]*bannerTrigger
}

type bannerTrigger struct {
	sound  sound.Name
	reason string
}

// triggerBanner schedules the session's banner, or merges into the one
// already waiting: attention beats done, and a reason replaces none.
func (s *Server) triggerBanner(id string, snd sound.Name, reason string) {
	b := &s.sessionBanners
	b.mu.Lock()
	defer b.mu.Unlock()
	if p := b.pending[id]; p != nil {
		if snd == sound.Attention {
			p.sound = snd
		}
		if reason != "" {
			p.reason = reason
		}
		return
	}
	if b.pending == nil {
		b.pending = map[string]*bannerTrigger{}
	}
	b.pending[id] = &bannerTrigger{sound: snd, reason: reason}
	time.AfterFunc(s.cfg.SessionBannerDelay, func() { s.fireBanner(id) })
}

func (s *Server) fireBanner(id string) {
	b := &s.sessionBanners
	b.mu.Lock()
	t := b.pending[id]
	delete(b.pending, id)
	b.mu.Unlock()
	afk, onScreen := s.afk.isOn(), s.focus.onScreen(id)
	if s.sessionWatches.isStopped() || (onScreen && !afk) {
		return
	}
	_, m, res, err := s.sessionDir(id)
	if res.Refused || err != nil {
		return
	}
	info := m.info()
	if quiet := info.Status.State == SessionWorking || info.Status.State == SessionStarting; quiet && m.Actionable.Reason == "" {
		return
	}
	title, body := bannerTitle(info), t.reason
	if body == "" {
		body = sessionStatusText(info)
	}
	// AFK adds chat without the focus check; the banner and sound keep theirs.
	if afk {
		s.chat.SessionAlert(info.Project, s.chatOverride(info.Project), title, body)
	}
	if onScreen {
		return
	}
	set := s.settings.Load()
	if set.AgentsBanners {
		s.sendSessionBanner(info, title, body)
	}
	if set.AgentsSounds.Enabled {
		go s.cfg.Player.Play(context.Background(), set.AgentsSounds, t.sound)
	}
}

func bannerTitle(info SessionInfo) string { return info.Project + " · " + info.Name }

func (s *Server) sendSessionBanner(info SessionInfo, title, body string) {
	term, connected := s.focus.clickTarget()
	note := osnotify.Notification{
		Title:    title,
		Body:     body,
		Group:    "gx-agent-" + info.ID,
		Activate: termBundleIDs[term],
	}
	if connected {
		note.Execute = "gx agents focus " + info.ID
	}
	if err := s.cfg.Notifier.Notify(context.Background(), note); err != nil {
		s.log.Warn("session banner", "session", info.ID, "err", err)
	}
}

// sessionStatusText reads like "Blocked: permission — Bash(rm -rf build)".
func sessionStatusText(info SessionInfo) string {
	st := info.Status
	labels := map[string]string{
		SessionIdle:        "Done",
		SessionBlocked:     "Blocked",
		SessionRateLimited: "Rate-limited",
		SessionExited:      "Exited",
	}
	text := labels[st.State]
	if text == "" {
		text = st.State
	}
	kind := st.Kind
	if st.State == SessionExited {
		kind = info.Exit
	}
	if kind != "" {
		text += ": " + kind
	}
	if st.Message != "" {
		text += " — " + st.Message
	}
	return text
}

// unreadSound is the sound of an Unread raised by a move to state.
func unreadSound(state string) sound.Name {
	if state == SessionBlocked {
		return sound.Attention
	}
	return sound.Done
}
