package claudeui

import (
	"cmp"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/transcript"
)

// NudgePrompt is the decision logged for each resend of a launch prompt.
const NudgePrompt NudgeDecision = "prompt-nudged"

// UnknownCommandMarker is the pane line claude draws for a skill its registry
// does not list (yet).
const UnknownCommandMarker = "Unknown command:"

// LaunchProof decides whether a launch command reached the agent. Its inputs
// are small funcs so a runner wires its own and a test fakes them.
//
// The proof is the session transcript: the command is delivered once the first
// user record written after the caller's pre-send count (see NewRecordSource)
// is the launch command. The pane only shortens the wait.
type LaunchProof struct {
	// FirstUser returns the content of the first user record written after
	// the caller's count taken before the first send; ok is false while there
	// is none.
	FirstUser func() (content string, ok bool, err error)
	// Working reports whether the runner shows the agent working; nil means
	// never. While it does the proof sends no resend.
	Working func() (bool, error)
	// CheckPasted (Claude only) fails a record that wraps the command in
	// Claude Code's pasted-content tag with ErrPastedLaunch.
	CheckPasted bool
	// UnknownMarker is the pane text that says the agent does not know the
	// skill; empty means Claude's UnknownCommandMarker.
	UnknownMarker string
	// Pane returns the current pane text.
	Pane func() (string, error)
	// Resend types the launch prompt again in the same session.
	Resend func() error
	// Report hears each resend; nil is allowed.
	Report func(NudgeDecision)
	Now    func() time.Time
	Sleep  func(time.Duration)
}

// LaunchProofCaps bound CheckLaunchDelivered. Grace is how long a missing user
// record keeps earning resends. UnknownGrace is the shorter window once the
// pane shows "Unknown command:": the registry is rarely about to fill, so the
// caller gets to retry in a fresh session sooner.
type LaunchProofCaps struct {
	Grace        time.Duration
	UnknownGrace time.Duration
	Poll         time.Duration
	ResendEvery  time.Duration
}

// WithOverrides returns c with every non-zero field of o replacing its own.
func (c LaunchProofCaps) WithOverrides(o LaunchProofCaps) LaunchProofCaps {
	c.Grace = cmp.Or(o.Grace, c.Grace)
	c.UnknownGrace = cmp.Or(o.UnknownGrace, c.UnknownGrace)
	c.Poll = cmp.Or(o.Poll, c.Poll)
	c.ResendEvery = cmp.Or(o.ResendEvery, c.ResendEvery)
	return c
}

// ErrPastedLaunch means the launch command reached the transcript as pasted
// text, so the skill did not run.
var ErrPastedLaunch = fmt.Errorf("%w: the launch command was pasted as plain text and the skill did not run", agentrunner.ErrNotDelivered)

const pastedMarker = "<pasted_content"

// DefaultLaunchProofCaps resend for about 30 seconds.
var DefaultLaunchProofCaps = LaunchProofCaps{
	Grace:        30 * time.Second,
	UnknownGrace: 5 * time.Second,
	Poll:         500 * time.Millisecond,
	ResendEvery:  3 * time.Second,
}

// NewRecordSource returns a LaunchProof.FirstUser over the transcript that
// find locates. It counts the person records there now, so call it before the
// first send: the source then returns the first record written after that
// count. A fresh session counts 0; a resumed one keeps its old records out of
// the proof.
func NewRecordSource(find func() (path string, ok bool, err error)) func() (string, bool, error) {
	texts := func() ([]string, error) {
		path, ok, err := find()
		if err != nil || !ok {
			return nil, err
		}
		msgs, _, err := transcript.ReadChat(path, "")
		if err != nil {
			return nil, err
		}
		var out []string
		for _, m := range msgs {
			if m.Role == transcript.ChatRolePerson {
				out = append(out, m.Text)
			}
		}
		return out, nil
	}
	before := 0
	if old, err := texts(); err == nil {
		before = len(old)
	}
	return func() (string, bool, error) {
		all, err := texts()
		if err != nil || len(all) <= before {
			return "", false, err
		}
		return all[before], true, nil
	}
}

// NewSessionRecordSource is NewRecordSource for a session known by id: while
// the id is empty there is no transcript to find, and find is not called.
func NewSessionRecordSource(sessionID string, find func() (path string, ok bool, err error)) func() (string, bool, error) {
	return NewRecordSource(func() (string, bool, error) {
		if sessionID == "" {
			return "", false, nil
		}
		return find()
	})
}

func firstPerson(path string) (string, bool, error) {
	msgs, _, err := transcript.ReadChat(path, "")
	if err != nil {
		return "", false, err
	}
	for _, m := range msgs {
		if m.Role == transcript.ChatRolePerson {
			return m.Text, true, nil
		}
	}
	return "", false, nil
}

// LaunchRecordedIn reports whether the first person record of the transcript at
// path is the launch command. known is false only when the transcript cannot be
// read; a readable one with no person message is known and not recorded (the
// session never got its task).
func LaunchRecordedIn(path, text string) (recorded, known bool, err error) {
	content, _, err := firstPerson(path)
	if err != nil {
		return false, false, err
	}
	return LaunchedBy(content, text), true, nil
}

// LaunchedBy reports whether a user record's content is the launch command:
// the command-name tag claude writes for a skill, or the typed text itself.
func LaunchedBy(content, text string) bool {
	name, _, _ := strings.Cut(strings.TrimSpace(text), " ")
	return strings.Contains(content, "<command-name>"+name+"</command-name>") ||
		strings.HasPrefix(strings.TrimSpace(content), strings.TrimSpace(text))
}

// CheckLaunchDelivered returns nil once the transcript shows text as the first
// user record, and agentrunner.ErrNotDelivered when the grace ends first. It
// resends text only while the transcript lacks the command, never after it
// shows it, and never while the agent is working, so a command never runs
// twice. A cancelled ctx ends the loop within
// one poll with the context error, not a delivery failure.
func CheckLaunchDelivered(ctx context.Context, p LaunchProof, text string, caps LaunchProofCaps) error {
	start := p.Now()
	var lastSend time.Time
	var unknownAt time.Time
	marker := cmp.Or(p.UnknownMarker, UnknownCommandMarker)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		content, ok, err := p.FirstUser()
		if err != nil {
			return fmt.Errorf("reading transcript: %w", err)
		}
		if ok && LaunchedBy(content, text) {
			return nil
		}
		if ok && p.CheckPasted && strings.Contains(content, pastedMarker) && strings.Contains(content, strings.TrimSpace(text)) {
			return fmt.Errorf("%w: %q", ErrPastedLaunch, text)
		}

		pane, err := p.Pane()
		if err != nil {
			return fmt.Errorf("reading pane: %w", err)
		}
		now := p.Now()
		if unknownAt.IsZero() && strings.Contains(pane, marker) {
			unknownAt = now
		}
		if !unknownAt.IsZero() && now.Sub(unknownAt) >= caps.UnknownGrace {
			return fmt.Errorf("%w: %w: %q: %s", agentrunner.ErrNotDelivered, agentrunner.ErrUnknownSkill, text, marker)
		}
		if now.Sub(start) >= caps.Grace {
			// The command still sits in the input and never reached the
			// transcript: the pane stopped taking keys.
			if holdsText(pane, text) {
				return &HungPaneError{Screen: pane}
			}
			return fmt.Errorf("%w: %q: no user record after %s", agentrunner.ErrNotDelivered, text, caps.Grace)
		}

		working := false
		if p.Working != nil {
			if working, err = p.Working(); err != nil {
				return fmt.Errorf("reading agent state: %w", err)
			}
		}
		if lastSend.IsZero() || now.Sub(lastSend) >= caps.ResendEvery {
			// The first pass is the caller's own send; only later passes
			// resend, and never into a working agent.
			if !lastSend.IsZero() && !working {
				if err := p.Resend(); err != nil {
					return fmt.Errorf("resending launch prompt: %w", err)
				}
				if p.Report != nil {
					p.Report(NudgePrompt)
				}
			}
			lastSend = now
		}
		p.Sleep(caps.Poll)
	}
}
