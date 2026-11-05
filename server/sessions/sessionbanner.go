package sessions

import (
	"context"
	"sync"
	"time"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/osnotify"
	"github.com/elentok/gx/sound"
	"github.com/elentok/gx/tickets"
)

const DefaultSessionBannerDelay = time.Second

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

// TriggerBanner schedules the session's banner, or merges into the one
// already waiting: attention beats done, and a reason replaces none.
func (mg *Manager) TriggerBanner(id string, snd sound.Name, reason string) {
	b := &mg.banners
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
	mg.Watches.Start(func() {
		delay := time.NewTimer(mg.cfg.SessionBannerDelay)
		defer delay.Stop()
		select {
		case <-delay.C:
			mg.fireBanner(id)
		case <-mg.Watches.Stop:
		}
	})
}

// NoteFailure triggers a banner when the session's failure is a new kind, and
// forgets it once the failure clears so a later one banners again. Auth and
// stalled come from the transcript, not the status, so they carry their label
// as the reason; a status failure already has its banner text.
func (mg *Manager) NoteFailure(id string) {
	_, m, res, err := mg.SessionDir(id)
	if res.Refused || err != nil {
		return
	}
	f := mg.host.SessionInfo(m).Failure
	kind := ""
	if f != nil {
		kind = f.Kind
	}
	b := &mg.banners
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
	if kind == wire.FailureKindAuth || kind == wire.FailureKindStalled {
		reason = f.Label
	}
	// A Done Design exiting cleanly is expected; the Design page hides it too.
	if kind == wire.FailureKindEnded && mg.designDone(m) {
		return
	}
	mg.TriggerBanner(id, sound.Attention, reason)
}

// designDone reports whether the session is a Design whose map's Progress is Done.
// mapProgress stays a root function; it moves with the map code.
func (mg *Manager) designDone(m Meta) bool {
	if !m.Design || m.Ticket == "" {
		return false
	}
	addr, err := tickets.ParseQueuedAddress(m.Ticket)
	if err != nil {
		return false
	}
	epics, err := mg.projects.Epics(addr.Project)
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
	step := queue.MapProgress(e)
	return step == wire.ProgressDone
}

// designLink is the Design page of a Design session, without the token; empty
// when the web listener is off or the session is not a Design.
func (mg *Manager) designLink(m Meta) string {
	base := mg.host.WebURL()
	if base == "" || !m.Design || m.Ticket == "" {
		return ""
	}
	addr, err := tickets.ParseQueuedAddress(m.Ticket)
	if err != nil {
		return ""
	}
	return base + "/#/d/" + queue.RootOf(addr).String()
}

func (mg *Manager) fireBanner(id string) {
	b := &mg.banners
	b.mu.Lock()
	t := b.pending[id]
	delete(b.pending, id)
	b.mu.Unlock()
	afk, onScreen := mg.afk.IsOn(), mg.focus.onScreen(id)
	if mg.Watches.IsStopped() || mg.focus.webOnScreen(id) || (onScreen && !afk) {
		return
	}
	_, m, res, err := mg.SessionDir(id)
	if res.Refused || err != nil {
		return
	}
	info := m.Info()
	if quiet := info.Status.State == wire.SessionWorking || info.Status.State == wire.SessionStarting; quiet && m.Actionable.Reason == "" && t.reason == "" {
		return
	}
	title, body := bannerTitle(info), t.reason
	if body == "" {
		body = sessionStatusText(info)
	}
	link := mg.designLink(m)
	// AFK adds chat without the focus check; the banner and sound keep theirs.
	if afk {
		chatBody := body
		if link != "" {
			chatBody += "\n" + link
		}
		mg.alerts.SessionAlert(info.Project, mg.projects.ChatOverride(info.Project), title, chatBody)
	}
	if onScreen {
		return
	}
	set := mg.settings.Load()
	if set.AgentsBanners {
		mg.sendSessionBanner(info, title, body, link)
	}
	if set.AgentsSounds.Enabled {
		mg.Watches.Start(func() { mg.cfg.Player.Play(context.Background(), set.AgentsSounds, t.sound) })
	}
}

func bannerTitle(info wire.SessionInfo) string { return info.Project + " · " + info.Name }

func (mg *Manager) sendSessionBanner(info wire.SessionInfo, title, body, link string) {
	term, connected := mg.focus.clickTarget()
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
	if err := mg.cfg.Notifier.Notify(context.Background(), note); err != nil {
		mg.log.Warn("session banner", "session", info.ID, "err", err)
	}
}

// sessionStatusText reads like "Blocked: permission — Bash(rm -rf build)".
func sessionStatusText(info wire.SessionInfo) string {
	st := info.Status
	labels := map[string]string{
		wire.SessionIdle:        "Done",
		wire.SessionBlocked:     "Blocked",
		wire.SessionRateLimited: "Rate-limited",
		wire.SessionExited:      "Exited",
	}
	text := labels[st.State]
	if text == "" {
		text = st.State
	}
	kind := st.Kind
	if st.State == wire.SessionExited {
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
	if state == wire.SessionBlocked {
		return sound.Attention
	}
	return sound.Done
}
