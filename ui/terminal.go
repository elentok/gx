package ui

import (
	"os"
	"runtime"
)

// Terminal identifies the terminal multiplexer or emulator gx is running inside.
type Terminal int

const (
	TerminalPlain       Terminal = iota // plain terminal, no multiplexer
	TerminalTmux                        // inside tmux ($TMUX set)
	TerminalKitty                       // inside kitty, no remote control ($KITTY_WINDOW_ID set)
	TerminalKittyRemote                 // inside kitty with remote control ($KITTY_LISTEN_ON set)
	TerminalHerdr                       // inside herdr ($HERDR_ENV set)
	TerminalGhostty                     // inside Ghostty on macOS ($TERM_PROGRAM=ghostty)
)

// String returns a short display label ("tmux", "kitty", "herdr") or "" for plain terminals.
func (t Terminal) String() string {
	switch t {
	case TerminalTmux:
		return "tmux"
	case TerminalKitty, TerminalKittyRemote:
		return "kitty"
	case TerminalHerdr:
		return "herdr"
	case TerminalGhostty:
		return "ghostty"
	default:
		return ""
	}
}

// CanSplit reports whether gx can open a new split pane for git commit.
func (t Terminal) CanSplit() bool {
	return t == TerminalTmux || t == TerminalKittyRemote || t == TerminalHerdr || t == TerminalGhostty
}

// DetectTerminal detects the current terminal environment from environment variables.
func DetectTerminal() Terminal {
	return DetectTerminalFrom(os.Getenv)
}

// DetectTerminalFrom detects the terminal environment using the provided getter
// and the current OS.
func DetectTerminalFrom(getenv func(string) string) Terminal {
	return DetectTerminalFor(getenv, runtime.GOOS)
}

// DetectTerminalFor is DetectTerminalFrom with an explicit GOOS. Ghostty is only
// detected on darwin (its split support is AppleScript-based); elsewhere it is plain.
func DetectTerminalFor(getenv func(string) string, goos string) Terminal {
	if getenv("HERDR_ENV") != "" {
		return TerminalHerdr
	}
	if getenv("KITTY_LISTEN_ON") != "" {
		return TerminalKittyRemote
	}
	if getenv("KITTY_WINDOW_ID") != "" {
		return TerminalKitty
	}
	if getenv("TMUX") != "" {
		return TerminalTmux
	}
	if goos == "darwin" && getenv("TERM_PROGRAM") == "ghostty" {
		return TerminalGhostty
	}
	return TerminalPlain
}
