package ui

import "testing"

func TestDetectTerminalPrefersTmuxOverKittyRemote(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,123,0")
	t.Setenv("KITTY_LISTEN_ON", "unix:/tmp/mykitty-70704")
	t.Setenv("KITTY_WINDOW_ID", "12")

	if got := DetectTerminal(); got != TerminalTmux {
		t.Fatalf("DetectTerminal() = %v, want %v", got, TerminalTmux)
	}
}

func TestDetectTerminalPrefersTmuxOverKittyWindow(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,123,0")
	t.Setenv("KITTY_LISTEN_ON", "")
	t.Setenv("KITTY_WINDOW_ID", "12")

	if got := DetectTerminal(); got != TerminalTmux {
		t.Fatalf("DetectTerminal() = %v, want %v", got, TerminalTmux)
	}
}

func TestDetectTerminalFallsBackToTmux(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,123,0")
	t.Setenv("KITTY_LISTEN_ON", "")
	t.Setenv("KITTY_WINDOW_ID", "")

	if got := DetectTerminal(); got != TerminalTmux {
		t.Fatalf("DetectTerminal() = %v, want %v", got, TerminalTmux)
	}
}

func TestDetectTerminalFrom(t *testing.T) {
	env := map[string]string{
		"TMUX":            "/tmp/tmux-1000/default,123,0",
		"KITTY_LISTEN_ON": "unix:/tmp/mykitty-70704",
	}

	getenv := func(key string) string {
		return env[key]
	}

	if got := DetectTerminalFrom(getenv); got != TerminalTmux {
		t.Fatalf("DetectTerminalFrom() = %v, want %v", got, TerminalTmux)
	}
}
