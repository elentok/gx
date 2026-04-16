package ui

import "os"

// Terminal identifies the terminal multiplexer or emulator gx is running inside.
type Terminal int

const (
	TerminalPlain       Terminal = iota // plain terminal, no multiplexer
	TerminalTmux                        // inside tmux ($TMUX set)
	TerminalKitty                       // inside kitty, no remote control ($KITTY_WINDOW_ID set)
	TerminalKittyRemote                 // inside kitty with remote control ($KITTY_LISTEN_ON set)
)

// String returns a short display label ("tmux", "kitty") or "" for plain terminals.
func (t Terminal) String() string {
	switch t {
	case TerminalTmux:
		return "tmux"
	case TerminalKitty, TerminalKittyRemote:
		return "kitty"
	default:
		return ""
	}
}

// CanSplit reports whether gx can open a new split pane for git commit.
func (t Terminal) CanSplit() bool {
	return t == TerminalTmux || t == TerminalKittyRemote
}

// DetectTerminal detects the current terminal environment from environment variables.
func DetectTerminal() Terminal {
	return DetectTerminalFrom(os.Getenv)
}

// DetectTerminalFrom detects the terminal environment using the provided getter.
func DetectTerminalFrom(getenv func(string) string) Terminal {
	if getenv("TMUX") != "" {
		return TerminalTmux
	}
	if getenv("KITTY_LISTEN_ON") != "" {
		return TerminalKittyRemote
	}
	if getenv("KITTY_WINDOW_ID") != "" {
		return TerminalKitty
	}
	return TerminalPlain
}
