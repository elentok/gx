package ralphloop

import (
	"fmt"

	"github.com/elentok/gx/chatmarkup"
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
// destination, shared by every project, so a notification goes out once no
// matter how many clients are connected.
type ServerChat struct {
	sinks []*chatEventSink
}

// NewServerChat starts a sink for each configured destination. It returns nil
// when none is configured; a nil *ServerChat is safe to use.
func NewServerChat(cfg ServerChatConfig) *ServerChat {
	c := &ServerChat{}
	if cfg.TelegramBotToken != "" {
		base := cfg.TelegramBaseURL
		if base == "" {
			base = telegramAPIBaseURL
		}
		c.sinks = append(c.sinks, newServerChatSink(telegramStyle, newTelegramTransport(cfg.TelegramBotToken, cfg.TelegramChatID, base), cfg.GateStatePath))
	}
	if cfg.SlackWebhookURL != "" {
		c.sinks = append(c.sinks, newServerChatSink(slackStyle, newSlackTransport(cfg.SlackWebhookURL), cfg.GateStatePath))
	}
	if len(c.sinks) == 0 {
		return nil
	}
	return c
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
func (c *ServerChat) Park(project, epic, ticketPath, identifier, status, reason string) {
	if c == nil {
		return
	}
	for _, s := range c.sinks {
		body := s.style.ticketNeedsHumanText(identifier, project+"/"+epic, status, reason, EpicCounts{})
		prefix := s.style.chatStyle.Escape(fmt.Sprintf("[%s] ", project))
		s.send(chatmarkup.Join(chatmarkup.Text{}, []chatmarkup.Text{prefix, body}), notifyKindTicketNeedsHuman, ticketPath, identifier)
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
	for _, s := range c.sinks {
		s.send(s.style.chatStyle.Message(n.Emoji, n.Title, "", n.Detail, s.style.identityLine("server", "")), n.Kind, serverSource, "")
	}
}

// Close flushes every queued batch, bounded by the transport timeout.
func (c *ServerChat) Close() {
	if c == nil {
		return
	}
	for _, s := range c.sinks {
		s.Close()
	}
}
