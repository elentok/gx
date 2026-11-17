// Package agentfake holds the fakes of an interactive CLI agent (Claude Code,
// Codex, ...): the worker that does a ticket's work (WorkTicket), the TUI
// fakes built on it (see cmd/ and tuiclaude) and the terminal titles they
// emit.
package agentfake

// Terminal-title OSC sequences: a half-circle spinner glyph for "working", an
// eight-spoked asterisk for "idle", each followed by a space, terminated with
// BEL.
const (
	IdleTitle    = "\x1b]0;✳ Claude Code\x07"
	WorkingTitle = "\x1b]0;◑ Claude Code\x07"
)
