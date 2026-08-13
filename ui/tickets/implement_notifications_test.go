package tickets

import (
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/testutil"
)

// startAndCaptureSink drives cmdStartImplement with notifications, capturing
// the EventSink runRalphLoop was actually called with.
func startAndCaptureSink(t *testing.T, notifications config.NotificationsConfig) ralphloop.EventSink {
	t.Helper()
	root := testutil.TempRepo(t)

	previousRun := runRalphLoop
	previousRegistry := ralphLoopRegistry
	captured := make(chan ralphloop.EventSink, 1)
	release := make(chan struct{})
	runRalphLoop = func(_ ralphloop.RunOptions, _ ralphloop.Deps, sink ralphloop.EventSink) error {
		captured <- sink
		<-release
		return nil
	}
	ralphLoopRegistry = newLoopRegistry(1)
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		runRalphLoop = previousRun
		ralphLoopRegistry = previousRegistry
	})

	cmd := cmdStartImplement(root, "alpha", ralphloop.AgentClaude, 0, 1, 1, nil, notifications, "gx-implement", config.AgentsConfig{})
	if msg, ok := cmd().(implementFailedMsg); ok {
		t.Fatalf("cmdStartImplement failed: %v", msg.err)
	}

	return <-captured
}

func TestCmdStartImplementWrapsSinkInTelegramDecoratorWhenBotTokenConfigured(t *testing.T) {
	// not parallel-safe: startAndCaptureSink reassigns the package-level runRalphLoop/ralphLoopRegistry singletons
	notifications := config.NotificationsConfig{Telegram: config.TelegramConfig{BotToken: "tok", ChatID: "42"}}
	sink := startAndCaptureSink(t, notifications)

	if _, ok := sink.(*ralphloop.ChannelEventSink); ok {
		t.Fatalf("expected sink wrapped in telegram decorator, got the bare ChannelEventSink")
	}
}

func TestCmdStartImplementWrapsSinkInSlackDecoratorWhenWebhookURLConfigured(t *testing.T) {
	// not parallel-safe: startAndCaptureSink reassigns the package-level runRalphLoop/ralphLoopRegistry singletons
	notifications := config.NotificationsConfig{Slack: config.SlackConfig{WebhookURL: "https://hooks.example.com/x"}}
	sink := startAndCaptureSink(t, notifications)

	if _, ok := sink.(*ralphloop.ChannelEventSink); ok {
		t.Fatalf("expected sink wrapped in slack decorator, got the bare ChannelEventSink")
	}
}

func TestCmdStartImplementUsesRealSinkDirectlyWhenNoNotificationsConfigured(t *testing.T) {
	// not parallel-safe: startAndCaptureSink reassigns the package-level runRalphLoop/ralphLoopRegistry singletons
	sink := startAndCaptureSink(t, config.NotificationsConfig{})

	if _, ok := sink.(*ralphloop.ChannelEventSink); !ok {
		t.Fatalf("expected the real sink unwrapped, got %T", sink)
	}
}

// TestCmdStartImplementChatSinksSatisfyChatEventSinkForShutdown pins ticket
// 13: the sink cmdStartImplement wires into runRalphLoop must be assertable
// against ralphloop.ChatEventSink so the shutdown goroutine's chatClosers
// cast (the interface{ Close() } this ticket replaced) keeps finding it —
// regardless of which decorator, or which order, wraps the run's base sink.
// Close() must also run without panicking (with nothing queued there's
// nothing to send over the network), the same call the shutdown goroutine
// makes for real.
func TestCmdStartImplementChatSinksSatisfyChatEventSinkForShutdown(t *testing.T) {
	// not parallel-safe: startAndCaptureSink reassigns the package-level runRalphLoop/ralphLoopRegistry singletons
	cases := []struct {
		name          string
		notifications config.NotificationsConfig
	}{
		{
			name:          "telegram only",
			notifications: config.NotificationsConfig{Telegram: config.TelegramConfig{BotToken: "tok", ChatID: "42"}},
		},
		{
			name:          "slack only",
			notifications: config.NotificationsConfig{Slack: config.SlackConfig{WebhookURL: "https://hooks.example.com/x"}},
		},
		{
			name: "telegram and slack",
			notifications: config.NotificationsConfig{
				Telegram: config.TelegramConfig{BotToken: "tok", ChatID: "42"},
				Slack:    config.SlackConfig{WebhookURL: "https://hooks.example.com/x"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sink := startAndCaptureSink(t, tc.notifications)

			closer, ok := sink.(ralphloop.ChatEventSink)
			if !ok {
				t.Fatalf("expected sink to satisfy ralphloop.ChatEventSink, got %T", sink)
			}
			closer.Close()
		})
	}
}
