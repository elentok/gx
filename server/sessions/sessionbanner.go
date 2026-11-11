package sessions

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/sessions/chat"
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
	// questionSent: this wait already sent its Question, which stands in for the AFK alert.
	questionSent bool
}

// TriggerBanner schedules the session's banner, or merges into the one
// already waiting: attention beats done, and a reason replaces none.
func (mg *Manager) TriggerBanner(id string, snd sound.Name, reason string) {
	triggerBanner(mg, id, snd, reason, false)
}

func triggerBanner(mg *Manager, id string, snd sound.Name, reason string, questionSent bool) {
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
		p.questionSent = p.questionSent || questionSent
		return
	}
	if b.pending == nil {
		b.pending = map[string]*bannerTrigger{}
	}
	b.pending[id] = &bannerTrigger{sound: snd, reason: reason, questionSent: questionSent}
	// Waits as a session watch, so close drops the banner and doesn't wait out the delay.
	mg.Watches.Start(func() {
		delay := time.NewTimer(mg.cfg.SessionBannerDelay)
		defer delay.Stop()
		select {
		case <-delay.C:
			fireBanner(mg, id)
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
	f := mg.SessionInfo(m).Failure
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
	if kind == wire.FailureKindEnded && designDone(mg.projects, m) {
		return
	}
	mg.TriggerBanner(id, sound.Attention, reason)
}

// designDone reports whether the session is a Design whose map's Progress is Done.
func designDone(reg *projects.Registry, m Meta) bool {
	step, ok := designStep(reg, m)
	return ok && step == wire.ProgressDone
}

// designStep is the Progress of the map behind a Design session.
func designStep(reg *projects.Registry, m Meta) (wire.ProgressStep, bool) {
	if !m.IsDesign() || m.Ticket == "" {
		return "", false
	}
	addr, err := tickets.ParseQueuedAddress(m.Ticket)
	if err != nil {
		return "", false
	}
	epics, err := reg.Epics(addr.Project)
	if err != nil {
		return "", false
	}
	for _, e := range epics {
		if e.Name == addr.Epic && e.IsMap {
			return queue.MapProgress(e), true
		}
	}
	return "", false
}

// DesignQuestion is the round a Design session waits on, as the Question
// message tells it. Text is the round's first question.
type DesignQuestion struct {
	Round int
	Text  string
}

// TriggerQuestionBanner is TriggerBanner for a Design that starts waiting on a
// Grilling round. It decides now, once, whether the person is told by chat: the
// Question goes out when the session is not open on screen and the round's mark
// is new, and then replaces the AFK alert of this wait's banner. Closing the
// session afterwards sends nothing more.
func (mg *Manager) TriggerQuestionBanner(id string, snd sound.Name, reason string, q DesignQuestion) {
	sent := sendDesignQuestion(mg, id, q)
	triggerBanner(mg, id, snd, reason, sent)
}

func sendDesignQuestion(mg *Manager, id string, q DesignQuestion) bool {
	if q.Round < 1 || q.Text == "" || mg.OpenOnScreen(id) {
		return false
	}
	_, m, res, err := mg.SessionDir(id)
	if res.Refused || err != nil {
		return false
	}
	step, ok := designStep(mg.projects, m)
	if !ok {
		return false
	}
	// A mark that cannot be taken sends nothing: a message may be lost, never doubled.
	// The round number restarts with each conversation (a handoff starts a new one
	// under the same session id), so the conversation is part of the key.
	isNew, err := mg.alerts.MarkOnce(fmt.Sprintf("question:%s:%s:%d", id, m.ConversationID, q.Round))
	if err != nil {
		mg.log.Warn("design question mark", "session", id, "err", err)
		return false
	}
	if !isNew {
		return false
	}
	addr, err := tickets.ParseQueuedAddress(m.Ticket)
	if err != nil {
		return false
	}
	project := m.Info().Project
	mg.alerts.DesignQuestion(project, mg.projects.ChatOverride(project), addr.Epic, string(step), q.Round, q.Text, designLink(mg.host.WebURL(), m))
	return true
}

// NoteQuestionCards sends one push for each dialog, Codex or permission card a
// plain chat newly shows, unless the session is open on screen. Other session
// classes send none: they have their own Question or banner.
func (mg *Manager) NoteQuestionCards(id string) {
	if mg.OpenOnScreen(id) {
		return
	}
	dir, m, res, err := mg.SessionDir(id)
	if res.Refused || err != nil {
		return
	}
	cm := m.ChatMeta()
	if chat.ClassOf(cm) != chat.ClassPlain {
		return
	}
	for _, card := range mg.chat.Cards(dir, cm, chat.Snapshot{}) {
		if card.Source != wire.CardSourceClaudeDialog && card.Source != wire.CardSourceCodex && card.Source != wire.CardSourcePermission {
			continue
		}
		// The permission id holds the turn, which an empty snapshot leaves at
		// zero, so the block's own start time tells two blocks of one dialog apart.
		key := fmt.Sprintf("chat-question:%s:%s:%s", id, card.Source, card.ID)
		if card.Source == wire.CardSourcePermission {
			key += fmt.Sprintf(":%d", m.StatusChangedAt.UnixNano())
		}
		// A mark that cannot be taken sends nothing: a message may be lost, never doubled.
		isNew, err := mg.alerts.MarkOnce(key)
		if err != nil {
			mg.log.Warn("chat question mark", "session", id, "err", err)
			continue
		}
		if !isNew {
			continue
		}
		info := m.Info()
		mg.alerts.ChatQuestion(info.Project, mg.projects.ChatOverride(info.Project), info.Name, cardText(card), designLink(mg.host.WebURL(), m))
	}
}

// cardText is the first question of a card, as the push tells it.
func cardText(card wire.QuestionCard) string {
	if len(card.Questions) == 0 {
		return ""
	}
	q := card.Questions[0]
	if q.Title != "" {
		return q.Title
	}
	return q.Body
}

// designLink is the Design page of a Design session, without the token; empty
// when the web listener is off or the session is not a Design.
func designLink(base string, m Meta) string {
	if base == "" || !m.IsDesign() || m.Ticket == "" {
		return ""
	}
	addr, err := tickets.ParseQueuedAddress(m.Ticket)
	if err != nil {
		return ""
	}
	return base + "/#/d/" + queue.RootOf(addr).String()
}

// OpenOnScreen reports whether the person already has the session open: a
// visible web tab always counts, a focused TUI only when AFK mode is off.
// The banner and the Design Question share it.
func (mg *Manager) OpenOnScreen(id string) bool {
	return mg.focus.webOnScreen(id) || (mg.focus.onScreen(id) && !mg.afk.IsOn())
}

func fireBanner(mg *Manager, id string) {
	b := &mg.banners
	b.mu.Lock()
	t := b.pending[id]
	delete(b.pending, id)
	b.mu.Unlock()
	afk, onScreen := mg.afk.IsOn(), mg.focus.onScreen(id)
	if mg.Watches.IsStopped() || mg.OpenOnScreen(id) {
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
	link := designLink(mg.host.WebURL(), m)
	// AFK adds chat without the focus check; the banner and sound keep theirs.
	if afk && !t.questionSent {
		chatBody := alertReason(info, t.reason)
		if link != "" {
			chatBody += "\n" + link
		}
		mg.alerts.SessionAlert(info.Project, mg.projects.ChatOverride(info.Project), info.Name, sessionStatusWord(info.Status.State), chatBody, info.Status.State != wire.SessionIdle)
	}
	if onScreen {
		return
	}
	set := mg.settings.Load()
	if set.AgentsBanners {
		sendSessionBanner(mg, info, title, body, link)
	}
	if set.AgentsSounds.Enabled {
		mg.Watches.Start(func() { mg.cfg.Player.Play(context.Background(), set.AgentsSounds, t.sound) })
	}
}

func bannerTitle(info wire.SessionInfo) string { return info.Project + " · " + info.Name }

func sendSessionBanner(mg *Manager, info wire.SessionInfo, title, body, link string) {
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

// sessionStatusWord is the Status word a state reads as; an unknown state
// reads as itself.
func sessionStatusWord(state string) string {
	labels := map[string]string{
		wire.SessionIdle:        "Done",
		wire.SessionBlocked:     "Blocked",
		wire.SessionRateLimited: "Rate-limited",
		wire.SessionExited:      "Exited",
	}
	if w := labels[state]; w != "" {
		return w
	}
	return state
}

// sessionStatusText reads like "Blocked: permission — Bash(rm -rf build)".
func sessionStatusText(info wire.SessionInfo) string {
	st := info.Status
	text := sessionStatusWord(st.State)
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

// alertReason is the line under an AFK alert's headline: the raised reason when
// there is one, else a plain sentence for the state. A blocked session keeps its
// kind and message.
func alertReason(info wire.SessionInfo, reason string) string {
	if reason != "" {
		return reason
	}
	st := info.Status
	switch st.State {
	case wire.SessionIdle:
		return "The agent finished and waits for you."
	case wire.SessionRateLimited:
		return "Claude hit its usage limit."
	case wire.SessionExited:
		return "The session ended."
	}
	return strings.TrimPrefix(sessionStatusText(info), sessionStatusWord(st.State)+": ")
}

// unreadSound is the sound of an Unread raised by a move to state.
func unreadSound(state string) sound.Name {
	if state == wire.SessionBlocked {
		return sound.Attention
	}
	return sound.Done
}
