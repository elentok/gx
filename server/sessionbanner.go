package server

import (
	"context"
	"sync"
	"time"

	"github.com/elentok/gx/osnotify"
	"github.com/elentok/gx/sound"
	"github.com/elentok/gx/tickets"
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
	// failing is the failure kind each failing session last raised a banner for.
	failing map[string]string
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
	// Waits as a session watch, so close drops the banner and doesn't wait out the delay.
	s.sessionWatches.start(func() {
		delay := time.NewTimer(s.cfg.SessionBannerDelay)
		defer delay.Stop()
		select {
		case <-delay.C:
			s.fireBanner(id)
		case <-s.sessionWatches.stop:
		}
	})
}

// noteFailure triggers a banner when the session's failure is a new kind, and
// forgets it once the failure clears so a later one banners again. Auth and
// stalled come from the transcript, not the status, so they carry their label
// as the reason; a status failure already has its banner text.
func (s *Server) noteFailure(id string) {
	_, m, res, err := s.sessionDir(id)
	if res.Refused || err != nil {
		return
	}
	f := s.sessionInfo(m).Failure
	kind := ""
	if f != nil {
		kind = f.Kind
	}
	b := &s.sessionBanners
	b.mu.Lock()
	prev := b.failing[id]
	if kind == "" {
		delete(b.failing, id)
	} else {
		if b.failing == nil {
			b.failing = map[string]string{}
		}
		b.failing[id] = kind
	}
	b.mu.Unlock()
	if kind == "" || kind == prev {
		return
	}
	reason := ""
	if kind == FailureKindAuth || kind == FailureKindStalled {
		reason = f.Label
	}
	// A Done Design exiting cleanly is expected; the Design page hides it too.
	if kind == FailureKindEnded && s.designDone(m) {
		return
	}
	s.triggerBanner(id, sound.Attention, reason)
}

// designDone reports whether the session is a Design whose map's Progress is Done.
func (s *Server) designDone(m sessionMeta) bool {
	if !m.Design || m.Ticket == "" {
		return false
	}
	addr, err := parseQueuedAddress(m.Ticket)
	if err != nil {
		return false
	}
	epics, err := s.projectEpics(addr.Project)
	if err != nil {
		return false
	}
	byName := map[string]tickets.Epic{}
	for _, e := range epics {
		byName[e.Name] = e
	}
	e, ok := byName[addr.Epic]
	if !ok || !e.IsMap {
		return false
	}
	step, _ := mapProgress(e, byName, s.mapProbeFor(addr.Project))
	return step == ProgressDone
}

// designLink is the Design page of a Design session, without the token; empty
// when the web listener is off or the session is not a Design.
func (s *Server) designLink(m sessionMeta) string {
	base := s.WebURL()
	if base == "" || !m.Design || m.Ticket == "" {
		return ""
	}
	addr, err := parseQueuedAddress(m.Ticket)
	if err != nil {
		return ""
	}
	return base + "/#/d/" + rootOf(addr).String()
}

func (s *Server) fireBanner(id string) {
	b := &s.sessionBanners
	b.mu.Lock()
	t := b.pending[id]
	delete(b.pending, id)
	b.mu.Unlock()
	afk, onScreen := s.afk.isOn(), s.focus.onScreen(id)
	if s.sessionWatches.isStopped() || s.focus.webOnScreen(id) || (onScreen && !afk) {
		return
	}
	_, m, res, err := s.sessionDir(id)
	if res.Refused || err != nil {
		return
	}
	info := m.info()
	if quiet := info.Status.State == SessionWorking || info.Status.State == SessionStarting; quiet && m.Actionable.Reason == "" && t.reason == "" {
		return
	}
	title, body := bannerTitle(info), t.reason
	if body == "" {
		body = sessionStatusText(info)
	}
	link := s.designLink(m)
	// AFK adds chat without the focus check; the banner and sound keep theirs.
	if afk {
		chatBody := body
		if link != "" {
			chatBody += "\n" + link
		}
		s.chat.SessionAlert(info.Project, s.chatOverride(info.Project), title, chatBody)
	}
	if onScreen {
		return
	}
	set := s.settings.Load()
	if set.AgentsBanners {
		s.sendSessionBanner(info, title, body, link)
	}
	if set.AgentsSounds.Enabled {
		s.sessionWatches.start(func() { s.cfg.Player.Play(context.Background(), set.AgentsSounds, t.sound) })
	}
}

func bannerTitle(info SessionInfo) string { return info.Project + " · " + info.Name }

func (s *Server) sendSessionBanner(info SessionInfo, title, body, link string) {
	term, connected := s.focus.clickTarget()
	note := osnotify.Notification{
		Title:    title,
		Body:     body,
		Group:    "gx-agent-" + info.ID,
		Activate: termBundleIDs[term],
		Link:     link,
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
