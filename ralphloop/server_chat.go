package ralphloop

import (
	"fmt"
	"strings"
	"sync"
)

// ServerChatConfig names the chat destinations a server notifies. An empty
// destination is off.
type ServerChatConfig struct {
	TelegramBotToken string
	TelegramChatID   string
	// TelegramBaseURL replaces the Bot API host; empty means the real one.
	TelegramBaseURL string
	SlackWebhookURL string
	// GateStatePath replaces the per-user notification gate state file; empty
	// means the real one.
	GateStatePath string
}

// ServerChat is the chat sink a server owns: one batcher and one gate per
// destination (transport + target), shared by every project, so a notification
// goes out once no matter how many clients are connected and two projects with
// the same target share one batch.
type ServerChat struct {
	base ServerChatConfig // carries the test seams a project destination reuses

	mu     sync.Mutex
	dests  map[string]*chatEventSink // destination key -> its sink
	global []*chatEventSink
	closed bool
}

// NewServerChat starts a sink for each configured global destination. It
// returns nil when none is configured; a nil *ServerChat is safe to use.
func NewServerChat(cfg ServerChatConfig) *ServerChat {
	c := &ServerChat{base: cfg, dests: map[string]*chatEventSink{}}
	c.global = c.sinksFor(cfg)
	if len(c.global) == 0 {
		return nil
	}
	return c
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
			s := newServerChatSink(telegramStyle, newTelegramTransport(cfg.TelegramBotToken, cfg.TelegramChatID, base), c.base.GateStatePath)
			s.gateKey = destinationKey(transportTelegram, target, telegramTarget(c.base.TelegramBotToken, c.base.TelegramChatID))
			return s
		})
	}
	if cfg.SlackWebhookURL != "" {
		add("slack\x00"+cfg.SlackWebhookURL, func() *chatEventSink {
			s := newServerChatSink(slackStyle, newSlackTransport(cfg.SlackWebhookURL), c.base.GateStatePath)
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
	if override == nil {
		return c.global
	}
	return c.sinksFor(*override)
}

func newServerChatSink(style mrkdwnStyle, transport chatTransport, gateStatePath string) *chatEventSink {
	// The server has no TUI to relay to and tracks the epic by address, so the
	// sink decorates a noop and carries no scratch dir or epic of its own.
	s := newChatEventSink(noopEventSink{}, style, transport, "", "")
	s.gateStatePath = gateStatePath
	s.startFlushLoop(batchFlushInterval)
	return s
}

// Park notifies that a person must look at a ticket the server could not land.
// Every message starts with the project name: one server notifies for many.
// override is the project's own notification block, nil to use the global one.
func (c *ServerChat) Park(project string, override *ServerChatConfig, epic, ticketPath, identifier, status, reason string, counts EpicCounts) {
	if c == nil {
		return
	}
	for _, s := range c.projectSinks(override) {
		body := s.style.ticketNeedsHumanText(identifier, project+"/"+epic, status, reason, counts)
		s.sendIn(s.style.chatStyle.Bold(project), body, notifyKindTicketNeedsHuman, ticketPath, identifier)
	}
}

// Escalated notifies that recovery gave a failure up to a person. detail names
// the failure, why recovery stopped, the matched entry and the report.
func (c *ServerChat) Escalated(project string, override *ServerChatConfig, epic, ticketPath, identifier, detail string) {
	if c == nil {
		return
	}
	// An epic-level escalation has no ticket to write a mute onto.
	source := ticketPath
	if source == "" {
		source = epicSource(project + "/" + epic)
	}
	for _, s := range c.projectSinks(override) {
		body := s.style.chatStyle.Message("⚠️", "recovery escalated", "", detail, s.style.identityLine(project+"/"+epic, identifier))
		s.sendIn(s.style.chatStyle.Bold(project), body, notifyKindTicketNeedsHuman, source, identifier)
	}
}

// EpicComplete tells the project's destinations that every ticket of epic
// landed and the epic's branch went onto its target.
func (c *ServerChat) EpicComplete(project string, override *ServerChatConfig, epic string, counts EpicCounts, elapsedSeconds int, totalCost float64) {
	if c == nil {
		return
	}
	for _, s := range c.projectSinks(override) {
		body := s.style.epicCompleteText(project+"/"+epic, counts, counts.Done, elapsedSeconds, totalCost)
		s.sendIn(s.style.chatStyle.Bold(project), body, notifyKindEpicComplete, epicSource(project+"/"+epic), "")
	}
}

// notifyKindResult tags the message a successful one-off sends on request.
const notifyKindResult = "result"

// maxResultRunes bounds the Result a chat message carries.
const maxResultRunes = 1500

// Result sends a one-off's ## Result, truncated, under its project. The
// server sink has no run log of its own, so the notification-sent event is
// written to the ticket's epic here, once per destination it was queued for.
func (c *ServerChat) Result(project string, override *ServerChatConfig, projectDir, epic, address, result string) {
	if c == nil {
		return
	}
	if r := []rune(result); len(r) > maxResultRunes {
		result = string(r[:maxResultRunes]) + "…"
	}
	for _, s := range c.projectSinks(override) {
		body := s.style.chatStyle.Message("✅", address+" finished", "", result, s.style.identityLine("server", ""))
		s.sendIn(s.style.chatStyle.Bold(project), body, notifyKindResult, address, "")
		logNotificationSent(projectDir, epic, s.transport.name(), notifyKindResult, body.String())
	}
}

// ParkDigest sends one message listing parks that were held back, each line
// carrying its own project name.
func (c *ServerChat) ParkDigest(lines []string) {
	if c == nil || len(lines) == 0 {
		return
	}
	title := fmt.Sprintf("%d parked while herdr was down", len(lines))
	for _, s := range c.global {
		s.send(s.style.chatStyle.Message("🅿️", title, "", strings.Join(lines, "\n"), s.style.identityLine("server", "")), notifyKindTicketNeedsHuman, serverSource, "")
	}
}

// ServerNotice is a server-level event: about the server itself, not a project.
type ServerNotice struct {
	Kind   string // notify kind, e.g. "server-started"
	Emoji  string
	Title  string
	Detail string
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
	for _, s := range c.global {
		s.send(s.style.chatStyle.Message(n.Emoji, n.Title, "", n.Detail, s.style.identityLine("server", "")), n.Kind, serverSource, "")
	}
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
