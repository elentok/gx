package main

import (
	"path/filepath"
	"slices"

	"github.com/elentok/gx/testutil/gxfixture/chatcapture"
)

// chatFlushInterval makes a message show in the capture within about a second.
const chatFlushInterval = "100ms"

// startChatCapture starts the capture endpoint for a ChatCapture scenario and
// points the server at it: the Slack webhook and a fake Telegram bot go in the
// config (through o.configs), the Telegram base URL and the short flush
// interval in the server env. `gx notify` and `gx config test-notifications`
// read the same config but always post Telegram to the real API host, so only
// their Slack sends reach the capture. The returned func stops the endpoint.
func (f *fixture) startChatCapture(o *opts, sc scenario) (func(), error) {
	if !sc.ChatCapture {
		return func() {}, nil
	}
	f.chatCapture = filepath.Join(f.root, "chat-capture.jsonl")
	srv, err := chatcapture.Start(f.chatCapture)
	if err != nil {
		return nil, err
	}
	o.configs = append(slices.Clone(o.configs),
		"notifications.slack.webhook-url="+srv.SlackURL(),
		"notifications.telegram.bot-token=123456:fake-token",
		"notifications.telegram.chat-id=\"424242\"",
	)
	flush := chatFlushInterval
	if sc.ChatFlush != "" {
		flush = sc.ChatFlush
	}
	f.env = append(f.env,
		"GX_CHAT_FLUSH_INTERVAL="+flush,
		"GX_CHAT_TELEGRAM_BASE_URL="+srv.TelegramBaseURL(),
	)
	return func() { _ = srv.Close() }, nil
}
