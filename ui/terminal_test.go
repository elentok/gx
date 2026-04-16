package ui

import "testing"

func TestDetectTerminalPrefersKittyRemoteOverTmux(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,123,0")
	t.Setenv("KITTY_LISTEN_ON", "unix:/tmp/mykitty-70704")
	t.Setenv("KITTY_WINDOW_ID", "12")

	if got := DetectTerminal(); got != TerminalKittyRemote {
		t.Fatalf("DetectTerminal() = %v, want %v", got, TerminalKittyRemote)
	}
}

func TestDetectTerminalPrefersKittyWindowOverTmux(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,123,0")
	t.Setenv("KITTY_LISTEN_ON", "")
	t.Setenv("KITTY_WINDOW_ID", "12")

	if got := DetectTerminal(); got != TerminalKitty {
		t.Fatalf("DetectTerminal() = %v, want %v", got, TerminalKitty)
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

	if got := DetectTerminalFrom(getenv); got != TerminalKittyRemote {
		t.Fatalf("DetectTerminalFrom() = %v, want %v", got, TerminalKittyRemote)
	}
}
