package notify

import (
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/notifykind"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/ralphloop/schedule"
)

// ServerChatConfig names the chat destinations a server notifies. An empty
// destination is off.
type ServerChatConfig struct {
	TelegramBotToken string
	TelegramChatID   string
	// TelegramBaseURL replaces the Bot API host; empty means the real one.
	TelegramBaseURL string
	SlackWebhookURL string
	// GateStatePath is the notification gate state file; the server fills it
	// from its state dir.
	GateStatePath string
	// FlushInterval replaces batchFlushInterval; zero means the real one.
	FlushInterval time.Duration
	// ProgressInterval replaces the Progress message interval; zero means the real one.
	ProgressInterval time.Duration
	// Quiet, when set and true, drops chat events on entry and at flush. It is
	// a seam, not a setting: a reload does not touch it. An interface rather
	// than a func so the config stays comparable.
	Quiet Quieter
	// Policy is the router's per-channel switch and kind list, read on every
	// send. Nil means every channel on and every kind checked. Like Quiet it is
	// a seam a reload does not touch, and an interface so the config stays
	// comparable (hold a pointer, never a slice).
	Policy ChannelPolicy
	// Desktop is the OS banner channel. Nil shows none. A seam like Policy.
	Desktop Desktop
	// Push is the Web Push channel. Nil sends none. A seam like Policy.
	Push Push
	// WebToast is the open-page toast channel. Nil shows none. A seam like Policy.
	WebToast WebToast
}

// Quieter is the predicate that silences chat.
type Quieter interface{ Quiet() bool }

func (s *chatEventSink) isQuiet() bool { return s.quiet != nil && s.quiet.Quiet() }

// ServerChat is the chat sink a server owns: one batcher and one gate per
// destination (transport + target), shared by every project, so a notification
// goes out once no matter how many clients are connected and two projects with
// the same target share one batch.
type ServerChat struct {
	mu     sync.Mutex
	base   ServerChatConfig          // carries the test seams a project destination reuses
	dests  map[string]*chatEventSink // destination key -> its sink
	global []*chatEventSink
	closed bool
}

// NewServerChat starts a sink for each configured global destination. With
// none configured nothing is sent, project overrides included, until SetGlobal
// names one. A nil *ServerChat is safe to use.
func NewServerChat(cfg ServerChatConfig) *ServerChat {
	c := &ServerChat{base: cfg, dests: map[string]*chatEventSink{}}
	c.global = c.sinksFor(cfg)
	return c
}

func (c *ServerChat) globalSinks() []*chatEventSink {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.global
}

// SetGlobal swaps the global destinations for the ones cfg names, keeping the
// test seams the chat was built with and every project sink with its batch
// and throttle state. It returns a func that closes the retired global sinks;
// Close waits on the network, so callers run it off their critical path.
func (c *ServerChat) SetGlobal(cfg ServerChatConfig) (closeRetired func()) {
	c.mu.Lock()
	c.base.TelegramBotToken, c.base.TelegramChatID, c.base.SlackWebhookURL = cfg.TelegramBotToken, cfg.TelegramChatID, cfg.SlackWebhookURL
	old := c.global
	c.mu.Unlock()
	next := c.sinksFor(cfg)

	c.mu.Lock()
	c.global = next
	var retired []*chatEventSink
	for key, s := range c.dests {
		if slices.Contains(old, s) && !slices.Contains(next, s) {
			delete(c.dests, key)
			retired = append(retired, s)
		}
	}
	c.mu.Unlock()
	return func() {
		for _, s := range retired {
			s.Close()
		}
	}
}

// sinksFor returns the sink of each destination cfg names, starting any that
// does not exist yet. Destinations are keyed by transport and target, so equal
// targets resolve to the same sink.
func (c *ServerChat) sinksFor(cfg ServerChatConfig) []*chatEventSink {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []*chatEventSink
	add := func(key string, start func() *chatEventSink) {
		s, ok := c.dests[key]
		if !ok {
			if c.closed {
				return
			}
			s = start()
			c.dests[key] = s
		}
		out = append(out, s)
	}
	if target := telegramTarget(cfg.TelegramBotToken, cfg.TelegramChatID); target != "" {
		add("telegram\x00"+target, func() *chatEventSink {
			base := firstNonEmptyStr(cfg.TelegramBaseURL, c.base.TelegramBaseURL, telegramAPIBaseURL)
			s := newServerChatSink(telegramStyle, newTelegramTransport(cfg.TelegramBotToken, cfg.TelegramChatID, base), c.base)
			s.gateKey = destinationKey(transportTelegram, target, telegramTarget(c.base.TelegramBotToken, c.base.TelegramChatID))
			return s
		})
	}
	if cfg.SlackWebhookURL != "" {
		add("slack\x00"+cfg.SlackWebhookURL, func() *chatEventSink {
			s := newServerChatSink(slackStyle, newSlackTransport(cfg.SlackWebhookURL), c.base)
			s.gateKey = destinationKey(transportSlack, cfg.SlackWebhookURL, slackTarget(c.base.SlackWebhookURL))
			return s
		})
	}
	return out
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// projectSinks picks where a project's notifications go: its override block
// replaces the global one whole (no per-transport merge), nil uses global.
func (c *ServerChat) projectSinks(override *ServerChatConfig) []*chatEventSink {
	global := c.globalSinks()
	if override == nil || len(global) == 0 {
		return global
	}
	return c.sinksFor(*override)
}

func newServerChatSink(style mrkdwnStyle, transport chatTransport, seams ServerChatConfig) *chatEventSink {
	// The server has no TUI to relay to and tracks the epic by address, so the
	// sink decorates a noop and carries no scratch dir or epic of its own.
	s := newChatEventSink(runlog.NoopEventSink{}, style, transport, "", "")
	s.gateStatePath = seams.GateStatePath
	s.quiet = seams.Quiet
	s.policy = seams.Policy
	interval := seams.FlushInterval
	if interval <= 0 {
		interval = batchFlushInterval
	}
	s.startFlushLoop(interval)
	return s
}

// Pause notifies that a person must look at a ticket the server could not merge.
// Every message starts with the project name: one server notifies for many.
// override is the project's own notification block, nil to use the global one.
func (c *ServerChat) Pause(project string, override *ServerChatConfig, epic, ticketPath, identifier, status string, kind events.Kind, reason string, counts schedule.EpicCounts) {
	c.pause(notifykind.PausedTicket, project, override, epic, ticketPath, identifier, status, reason)
}

// RootPause is Pause for an all-done root the merge could not finish: the
// catalog counts it as an epic pause.
func (c *ServerChat) RootPause(project string, override *ServerChatConfig, epic, ticketPath, identifier, status, reason string) {
	c.pause(notifykind.EpicPaused, project, override, epic, ticketPath, identifier, status, reason)
}

func (c *ServerChat) pause(catalog, project string, override *ServerChatConfig, epic, ticketPath, identifier, status, reason string) {
	if c == nil {
		return
	}
	msg := ticketPauseMessage(project, epic, ticketPath, identifier, status, reason).as(catalog)
	target := ticketTarget(project, epic, identifier)
	if catalog == notifykind.EpicPaused {
		target = ticketTarget(project, epic, "")
	}
	c.show(msg, target)
	for _, s := range c.projectSinks(override) {
		s.sendMessage(msg, ticketPath, identifier)
	}
}

// EpicPause notifies that epic waits on a person for a pause of its own, not a
// ticket's: label names it and reason says why.
func (c *ServerChat) EpicPause(project string, override *ServerChatConfig, epic, label, reason string) {
	if c == nil {
		return
	}
	msg := epicPauseMessage(project, epic, label, reason).as(notifykind.EpicPaused)
	c.show(msg, ticketTarget(project, epic, ""))
	for _, s := range c.projectSinks(override) {
		s.sendMessage(msg, epicSource(project+"/"+epic), "")
	}
}

// FinishSink is the EventSink the server's finish path reports through for
// one ticket. It forwards only what the server cannot see from the outcome: a
// conflict resolution starting, and the resolution child pausing. The ticket's
// own pause is dropped, since the server notifies it from the outcome.
func (c *ServerChat) FinishSink(project string, override *ServerChatConfig, epic, ticketPath, ticketID string, counts func() schedule.EpicCounts) runlog.EventSink {
	return finishSink{c: c, project: project, override: override, epic: epic, ticketPath: ticketPath, ticketID: ticketID, counts: counts}
}

type finishSink struct {
	runlog.NoopEventSink
	c                          *ServerChat
	project                    string
	override                   *ServerChatConfig
	epic, ticketPath, ticketID string
	counts                     func() schedule.EpicCounts
}

func (f finishSink) ConflictResolutionStarted(identifier string) {
	if f.c == nil {
		return
	}
	msg := conflictStartedMessage(f.project, f.epic, f.ticketPath, identifier).as(notifykind.ConflictResolutionStarted)
	f.c.show(msg, ticketTarget(f.project, f.epic, identifier))
	for _, s := range f.c.projectSinks(f.override) {
		s.sendMessage(msg, f.ticketPath, identifier)
	}
}

func (f finishSink) TicketNeedsHuman(identifier, _, status string, kind events.Kind, reason string) {
	if f.c == nil || identifier == f.ticketID {
		return
	}
	// The child's path never reaches the sink, so a mute merges on the epic.
	source := epicSource(f.project + "/" + f.epic)
	// The child sits beside the parent ticket: <scratch>/<epic>/issues/<id>-*.md.
	epicDir := filepath.Dir(filepath.Dir(f.ticketPath))
	childPath := ResolveTicketPath(filepath.Dir(epicDir), filepath.Base(epicDir), identifier)
	msg := ticketPauseMessage(f.project, f.epic, childPath, identifier, status, reason).as(notifykind.PausedTicket)
	f.c.show(msg, ticketTarget(f.project, f.epic, identifier))
	for _, s := range f.c.projectSinks(f.override) {
		s.sendMessage(msg, source, identifier)
	}
}

// Escalated notifies that recovery gave a failure up to a person. failure is
// the failure's pause sentence, empty for an epic-level escalation.
func (c *ServerChat) Escalated(project string, override *ServerChatConfig, epic, ticketPath, identifier, failure string) {
	if c == nil {
		return
	}
	// An epic-level escalation has no ticket to write a mute onto.
	source := ticketPath
	if source == "" {
		source = epicSource(project + "/" + epic)
	}
	msg := escalatedMessage(project, epic, ticketPath, identifier, failure).as(notifykind.PausedTicket)
	target := ticketTarget(project, epic, identifier)
	if ticketPath == "" {
		target = ticketTarget(project, epic, "")
	}
	c.showUrgent(msg, target, true)
	for _, s := range c.projectSinks(override) {
		s.sendMessage(msg, source, identifier)
	}
}

// EpicComplete tells the project's destinations that every ticket of epic is
// done: Merged when its branch went onto its target, Complete otherwise.
func (c *ServerChat) EpicComplete(project string, override *ServerChatConfig, epic string, counts schedule.EpicCounts, elapsedSeconds int, totalCost float64, merged bool) {
	if c == nil {
		return
	}
	msg := epicOutcomeMessage(project, epic, counts, elapsedSeconds, totalCost, merged).as(notifykind.EpicComplete)
	c.show(msg, ticketTarget(project, epic, ""))
	for _, s := range c.projectSinks(override) {
		s.sendMessage(msg, epicSource(project+"/"+epic), "")
	}
}

// Progress sends one progress update for epic to the project's destinations.
func (c *ServerChat) Progress(project string, override *ServerChatConfig, epic string, facts ProgressFacts) {
	if c == nil {
		return
	}
	msg := progressMessage(project, epic, facts).as(notifykind.ProgressUpdate)
	c.show(msg, ticketTarget(project, epic, ""))
	for _, s := range c.projectSinks(override) {
		s.sendMessage(msg, epicSource(project+"/"+epic), "")
	}
}

// DesignQuestion tells the project's destinations that the Design epic waits
// at step for an answer to the round's first question.
func (c *ServerChat) DesignQuestion(project string, override *ServerChatConfig, epic, step string, round int, question, link string) {
	if c == nil {
		return
	}
	msg := designQuestionMessage(project, epic, step, round, question, link).as(notifykind.DesignQuestion)
	for _, s := range c.projectSinks(override) {
		s.sendMessage(msg, epicSource(project+"/"+epic), "")
	}
}

// ChatQuestion tells the project's destinations that a plain chat session waits
// on a dialog, Codex or permission card. Like DesignQuestion it is never
// per-source muted; the source is the server.
func (c *ServerChat) ChatQuestion(project string, override *ServerChatConfig, session, question, link string) {
	if c == nil {
		return
	}
	msg := chatQuestionMessage(project, session, question, link).as(notifykind.ChatQuestion)
	c.show(msg, Target{})
	for _, s := range c.projectSinks(override) {
		s.sendMessage(msg, serverSource, "")
	}
}

// DesignReady tells the project's destinations that the Design of a map
// finished and its implEpic holds tickets ready to queue. The source is the
// Design's own epic, the same as its Question.
func (c *ServerChat) DesignReady(project string, override *ServerChatConfig, designEpic, implEpic string, count int, link string) {
	if c == nil {
		return
	}
	msg := designReadyMessage(project, implEpic, count, link).as(notifykind.DesignReady)
	c.show(msg, Target{Design: project + ":" + designEpic})
	for _, s := range c.projectSinks(override) {
		s.sendMessage(msg, epicSource(project+"/"+designEpic), "")
	}
}

// MarkOnce records key in the server's gate state and reports whether it was
// new. An error or a nil chat reports false: a message may be lost, never doubled.
func (c *ServerChat) MarkOnce(key string) (bool, error) {
	if c == nil {
		return false, errNoNotificationStatePath
	}
	c.mu.Lock()
	path := c.base.GateStatePath
	c.mu.Unlock()
	return MarkOnce(path, key)
}

// notifyKindResult tags the message a successful one-off sends on request.
const notifyKindResult = "result"

// Result sends a one-off's ## Result, truncated, under its project. The
// server sink has no run log of its own, so the notification-sent event is
// written to the ticket's epic here, once per destination it was queued for.
func (c *ServerChat) Result(project string, override *ServerChatConfig, projectDir, epic, ticketPath, identifier, result string) {
	if c == nil {
		return
	}
	msg := resultMessage(project, epic, ticketPath, identifier, result).as(notifykind.OneOffResult)
	c.show(msg, ticketTarget(project, epic, identifier))
	for _, s := range c.projectSinks(override) {
		s.sendMessage(msg, ticketPath, identifier)
		logNotificationSent(projectDir, epic, s.transport.name(), notifyKindResult, msg.Text(s.style.chatStyle).String())
	}
}

// ServerNotice is a server-level event: about the server itself, not a project.
type ServerNotice struct {
	Kind   string // notify kind, e.g. "server-started"
	Emoji  string
	Title  string
	Detail string
	// HoldsWork marks a notice that means work is stopped: hard budget limit,
	// low disk, trunk missing.
	HoldsWork bool
	// Target is the page a Desktop banner opens; the zero value is the app home.
	Target Target
}

// serverSource is the gate source for server-level notices; like "cli" it has
// no ticket to write a mute onto.
const serverSource = "server"

// Notice sends a server-level event to the global destination only. No project
// prefix: it belongs to no project.
func (c *ServerChat) Notice(n ServerNotice) {
	if c == nil {
		return
	}
	msg := serverNoticeMessage(n).as(noticeCatalogID(n.Kind))
	c.show(msg, n.Target)
	for _, s := range c.globalSinks() {
		s.sendMessage(msg, serverSource, "")
	}
}

// NotifyOutcomes sends one line of text to the global destination, through the
// same gate as `gx notify`: source and kind are the CLI's, so its mutes apply.
// It says what each channel did.
func (c *ServerChat) NotifyOutcomes(text string) []ChannelOutcome {
	return c.Route(chatMessage{subject: text, kind: notifyKindCLI, summaryWord: "Message", catalog: notifykind.CLIMessage}, Target{}, false)
}

// TestNotifications sends a test message as a system message on all five
// channels: it skips the kind lists, and a channel whose switch is off stays silent.
func (c *ServerChat) TestNotifications() []ChannelOutcome {
	return c.Route(testMessage(), Target{}, true)
}

// notifyKindSessionAlert tags an interactive session's banner sent to chat in
// AFK mode.
const notifyKindSessionAlert = "session-alert"

// SessionAlert sends an interactive session's alert, "🔔 <status> <name>" with
// the reason below and the project last, to the project's destinations. The
// gate source is the server's: a session has no ticket to write a mute onto.
// needsAttention marks the session as stuck (Blocked, Rate-limited, Exited)
// rather than done, which makes the alert urgent.
func (c *ServerChat) SessionAlert(project string, override *ServerChatConfig, name, status, body string, needsAttention bool) {
	if c == nil {
		return
	}
	msg := sessionAlertMessage(project, name, status, body, needsAttention)
	for _, s := range c.projectSinks(override) {
		s.sendMessage(msg, serverSource, "")
	}
}

func sessionAlertMessage(project, name, status, body string, needsAttention bool) chatMessage {
	msg := chatMessage{icon: "🔔", status: status, subject: name, lines: []string{body}, project: project, kind: notifyKindSessionAlert, catalog: notifykind.SessionAlert}
	if needsAttention {
		msg.urgency = urgencyAnswer
	}
	return msg
}

// Close flushes every queued batch, bounded by the transport timeout.
func (c *ServerChat) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.closed = true
	sinks := make([]*chatEventSink, 0, len(c.dests))
	for _, s := range c.dests {
		sinks = append(sinks, s)
	}
	c.mu.Unlock()
	for _, s := range sinks {
		s.Close()
	}
}
