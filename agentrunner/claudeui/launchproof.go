package claudeui

import (
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
// The proof is the session transcript: the command is delivered once its first
// user record is the launch command. The pane only shortens the wait.
type LaunchProof struct {
	// FirstUser returns the content of the transcript's first user record;
	// ok is false while there is none.
	FirstUser func() (content string, ok bool, err error)
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

// DefaultLaunchProofCaps resend for about 30 seconds.
var DefaultLaunchProofCaps = LaunchProofCaps{
	Grace:        30 * time.Second,
	UnknownGrace: 5 * time.Second,
	Poll:         500 * time.Millisecond,
	ResendEvery:  3 * time.Second,
}

// FirstUserRecord is the text of the first person message of the transcript of
// sessionID under home; ok is false until the transcript exists and has one.
func FirstUserRecord(home, sessionID string) (string, bool, error) {
	if sessionID == "" {
		return "", false, nil
	}
	paths, err := transcript.FindByID(home, sessionID)
	if err != nil || len(paths) != 1 {
		return "", false, err
	}
	msgs, _, err := transcript.ReadChat(paths[0])
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

// LaunchRecorded reports whether the transcript of sessionID under home
// already shows text as its first user record.
func LaunchRecorded(home, sessionID, text string) (bool, error) {
	content, ok, err := FirstUserRecord(home, sessionID)
	return ok && launchedBy(content, text), err
}

// launchedBy reports whether a user record's content is the launch command:
// the command-name tag claude writes for a skill, or the typed text itself.
func launchedBy(content, text string) bool {
	name, _, _ := strings.Cut(strings.TrimSpace(text), " ")
	return strings.Contains(content, "<command-name>"+name+"</command-name>") ||
		strings.HasPrefix(strings.TrimSpace(content), strings.TrimSpace(text))
}

// CheckLaunchDelivered returns nil once the transcript shows text as the first
// user record, and agentrunner.ErrNotDelivered when the grace ends first. It
// resends text only while the transcript lacks the command, never after it
// shows it, so a command never runs twice. A cancelled ctx ends the loop within
// one poll with the context error, not a delivery failure.
func CheckLaunchDelivered(ctx context.Context, p LaunchProof, text string, caps LaunchProofCaps) error {
	start := p.Now()
	var lastSend time.Time
	var unknownAt time.Time
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		content, ok, err := p.FirstUser()
		if err != nil {
			return fmt.Errorf("reading transcript: %w", err)
		}
		if ok && launchedBy(content, text) {
			return nil
		}

		pane, err := p.Pane()
		if err != nil {
			return fmt.Errorf("reading pane: %w", err)
		}
		now := p.Now()
		if unknownAt.IsZero() && strings.Contains(pane, UnknownCommandMarker) {
			unknownAt = now
		}
		if !unknownAt.IsZero() && now.Sub(unknownAt) >= caps.UnknownGrace {
			return fmt.Errorf("%w: %w: %q: %s", agentrunner.ErrNotDelivered, agentrunner.ErrUnknownSkill, text, UnknownCommandMarker)
		}
		if now.Sub(start) >= caps.Grace {
			return fmt.Errorf("%w: %q: no user record for the command after %s", agentrunner.ErrNotDelivered, text, caps.Grace)
		}

		if lastSend.IsZero() || now.Sub(lastSend) >= caps.ResendEvery {
			// The first pass is the caller's own send; only later passes resend.
			if !lastSend.IsZero() {
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
