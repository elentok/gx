package tickets

import (
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/testutil"
)

// startAndCaptureSink drives cmdStartImplement with telegram, capturing the
// EventSink runRalphLoop was actually called with.
func startAndCaptureSink(t *testing.T, telegram config.TelegramConfig) ralphloop.EventSink {
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

	cmd := cmdStartImplement(root, "alpha", ralphloop.AgentClaude, 0, 1, 1, nil, telegram)
	if msg, ok := cmd().(implementFailedMsg); ok {
		t.Fatalf("cmdStartImplement failed: %v", msg.err)
	}

	return <-captured
}

func TestCmdStartImplementWrapsSinkInTelegramDecoratorWhenBotTokenConfigured(t *testing.T) {
	sink := startAndCaptureSink(t, config.TelegramConfig{BotToken: "tok", ChatID: "42"})

	if _, ok := sink.(*ralphloop.ChannelEventSink); ok {
		t.Fatalf("expected sink wrapped in telegram decorator, got the bare ChannelEventSink")
	}
}

func TestCmdStartImplementUsesRealSinkDirectlyWhenNoBotTokenConfigured(t *testing.T) {
	sink := startAndCaptureSink(t, config.TelegramConfig{})

	if _, ok := sink.(*ralphloop.ChannelEventSink); !ok {
		t.Fatalf("expected the real sink unwrapped, got %T", sink)
	}
}
