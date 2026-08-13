// Package agentfake implements a small controllable stand-in for an
// interactive CLI agent (Claude Code, Codex, ...), built as the literal
// binary name "claude" (see cmd/agentfake) so `herdr agent start --kind
// claude` finds it on PATH in place of a real agent. It emits the same
// terminal-title patterns herdr's claude.toml detection manifest scrapes to
// decide whether a pane is idle or working, with knobs for the timing
// scenarios the herdr-e2e-testing map needs to regression-test: an
// immediately-idle agent, a slow-working one, a fake long-`/compact`-style
// pause triggered by a prompt, and a stalled one that never signals a state
// change after a prompt.
//
// This is the agent-process side of the fake; testutil/herdrfake fakes the
// other side (herdr itself) for in-process unit tests. agentfake is driven
// through a real herdr daemon instead, from e2e/.
package agentfake

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"time"
)

// Modes accepted by --mode.
const (
	// ModeIdle emits the idle title once and then ignores all input.
	ModeIdle = "idle"
	// ModeSlowWorking emits the working title, sleeps for --duration, then
	// emits the idle title and ignores all further input.
	ModeSlowWorking = "slow-working"
	// ModeCompact starts idle; each input line (a submitted prompt) triggers
	// a working title, a sleep for --duration (the fake compaction pause),
	// then the idle title again — simulating a long `/compact`-style turn.
	ModeCompact = "compact"
	// ModeStall starts idle; an input line produces no title change at all,
	// simulating a prompt that never observably starts — for testing
	// herdr's agent_prompt_stalled behavior.
	ModeStall = "stall"
)

// Terminal-title OSC sequences matching claude.toml's osc_title_idle
// (`^\x{2733} `) and osc_title_working (`^[\x{2800}-\x{28FF}\x{25D0}-\x{25D1}] `)
// rules — a half-circle spinner glyph for "working", an eight-spoked
// asterisk for "idle", each followed by a space, terminated with BEL.
const (
	idleTitle    = "\x1b]0;✳ Claude Code\x07"
	workingTitle = "\x1b]0;◑ Claude Code\x07"
)

// Options are the parsed --mode/--duration flags.
type Options struct {
	Mode     string
	Duration time.Duration
}

// ParseArgs parses agentfake's command-line flags. Unrecognized flags
// (herdr's own --permission-mode-style agent args, if ever passed) are
// rejected the same as any other unknown flag, since agentfake accepts only
// its own scenario knobs.
func ParseArgs(args []string) (Options, error) {
	fs := flag.NewFlagSet("agentfake", flag.ContinueOnError)
	mode := fs.String("mode", ModeIdle, "idle | slow-working | compact | stall")
	duration := fs.Duration("duration", 10*time.Second, "working/compaction pause duration")
	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}
	switch *mode {
	case ModeIdle, ModeSlowWorking, ModeCompact, ModeStall:
	default:
		return Options{}, fmt.Errorf("agentfake: unknown --mode %q", *mode)
	}
	return Options{Mode: *mode, Duration: *duration}, nil
}

// Run drives the fake agent's behavior for opts.Mode, reading submitted
// prompts (one per line) from in and writing terminal-title escape
// sequences to out, until in reaches EOF (the pane is torn down).
func Run(opts Options, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)

	switch opts.Mode {
	case ModeIdle:
		fmt.Fprint(out, idleTitle)
		for scanner.Scan() {
			// Idle agent: input is received but never changes state.
		}
		return nil

	case ModeSlowWorking:
		fmt.Fprint(out, workingTitle)
		time.Sleep(opts.Duration)
		fmt.Fprint(out, idleTitle)
		for scanner.Scan() {
		}
		return nil

	case ModeCompact:
		fmt.Fprint(out, idleTitle)
		for scanner.Scan() {
			fmt.Fprint(out, workingTitle)
			time.Sleep(opts.Duration)
			fmt.Fprint(out, idleTitle)
		}
		return nil

	case ModeStall:
		fmt.Fprint(out, idleTitle)
		for scanner.Scan() {
			// Stalled agent: input received, no title change ever emitted.
		}
		return nil

	default:
		return fmt.Errorf("agentfake: unknown mode %q", opts.Mode)
	}
}
