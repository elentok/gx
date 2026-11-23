package deliver

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner/holder"
)

// ErrCancelled means the caller's Cancel channel closed: no more keys go out.
var ErrCancelled = errors.New("nudge cancelled: person input")

// imageChipWait bounds how long Submit waits for pasted images to show as
// "[Image #N]" chips before it types the text.
//
// Measured against a real claude: the chip took 8s and 19s to draw after a
// bracketed path paste in two of three tries, and keys sent meanwhile land
// before it and lose the Enter.
const imageChipWait = 30 * time.Second

// Target is the one production Prompter: the keys and screen reads over a
// session's holder. The harness decides how text is typed (Typing). What is
// not typing is the caller's: the lock, the cancel channel, the callbacks and
// the start signal.
type Target struct {
	C      *holder.HolderClient
	Typing harness.Typing
	IO     harness.TypingIO
	// Cancel, once closed, stops every key with ErrCancelled.
	Cancel <-chan struct{}
	// OnTyped, when set, runs once the text is typed.
	OnTyped func()
	// Start is WaitStart: false, not an error, when timeout passes with no turn.
	Start func(timeout time.Duration) (bool, error)
	// SkipType makes the next Submit press the submit key without typing: the
	// text is already in the input (claude's --prefill). A later Submit, after
	// a clear, types as usual.
	SkipType bool
	// Images are file paths pasted before the text on every Submit, so a retype
	// after a clear attaches them again. ImageSent runs with an image's index
	// once its own paste succeeds.
	Images    []string
	ImageSent func(i int)
	// Lock takes the chat's typing lock; Submit holds it for its type and Enter.
	// Nil takes none.
	Lock func() (func(), error)
	// Held is a lock the caller already took; the next Submit uses it instead of
	// taking its own. Submit and the caller both release it, once.
	Held func()
}

// ReleaseHeld releases the caller's Held lock, once.
func (t *Target) ReleaseHeld() {
	if release := t.Held; release != nil {
		t.Held = nil
		release()
	}
}

// acquire returns the release func of the typing lock for one Submit, with gx
// the input owner for the batch.
func (t *Target) acquire() (func(), error) {
	if t.Held != nil {
		return t.ReleaseHeld, nil
	}
	release := func() {}
	if t.Lock != nil {
		var err error
		if release, err = t.Lock(); err != nil {
			return nil, err
		}
	}
	return HoldInput(t.C, release)
}

// HoldInput makes gx the input owner for one batch. The returned func ends the
// batch, then runs release; a failed begin runs release itself.
func HoldInput(c *holder.HolderClient, release func()) (func(), error) {
	if err := c.BatchBegin(); err != nil {
		release()
		return nil, err
	}
	return func() {
		_ = c.BatchEnd()
		release()
	}, nil
}

func (t *Target) cancelled() error {
	select {
	case <-t.Cancel:
		return ErrCancelled
	default:
		return nil
	}
}

// Submit types text and presses the submit key. The caller strips text first
// (Deliver does).
func (t *Target) Submit(text string) error {
	release, err := t.acquire()
	if err != nil {
		return err
	}
	defer release()
	if err := t.typeLocked(text); err != nil {
		return err
	}
	return t.submitKey()
}

// Type types text, with the images pasted before it, and presses nothing.
func (t *Target) Type(text string) error {
	release, err := t.acquire()
	if err != nil {
		return err
	}
	defer release()
	return t.typeLocked(text)
}

// TypeThenSubmit types text and presses the submit key under one lock and
// batch; between runs after the typing, before the key.
func (t *Target) TypeThenSubmit(text string, between func()) error {
	release, err := t.acquire()
	if err != nil {
		return err
	}
	defer release()
	if err := t.typeLocked(text); err != nil {
		return err
	}
	between()
	return t.submitKey()
}

// ImageCount is how many images Type pastes before the text.
func (t *Target) ImageCount() int { return len(t.Images) }

// ReadRows is the plain screen rows and whether the read is untrusted: taken
// after a PTY resize and before the child's next output. A holder without
// styled rows serves plain rows, never resized.
func (t *Target) ReadRows() ([]string, bool, error) {
	if !t.C.Supports(holder.OpScreenStyled) {
		rows, _, err := t.C.Screen(0)
		return rows, false, err
	}
	rows, read, err := t.C.StyledScreenRead()
	return harness.PlainRows(rows), read.Resized, err
}

// typeLocked is the typing half of Submit and Type, with the lock held.
func (t *Target) typeLocked(text string) error {
	if err := t.cancelled(); err != nil {
		return err
	}
	if t.SkipType {
		t.SkipType = false
		if t.OnTyped != nil {
			t.OnTyped()
		}
		return nil
	}
	for i, path := range t.Images {
		if err := t.C.TypeTextBy(path, true); err != nil {
			return err
		}
		if t.ImageSent != nil {
			t.ImageSent(i)
		}
	}
	t.waitImageChips(len(t.Images))
	if len(t.Images) > 0 {
		text = " " + text
	}
	if err := t.C.TypeTextBy(text, t.Typing.PastesText(text)); err != nil {
		return err
	}
	if t.OnTyped != nil {
		t.OnTyped()
	}
	return nil
}

// waitImageChips waits until the screen shows n image chips. Claude reads a
// pasted path off disk before it draws the chip, and text typed or an Enter
// pressed meanwhile can be lost. A screen that never shows them (another
// harness) costs imageChipWait, then typing goes on.
func (t *Target) waitImageChips(n int) {
	if n == 0 {
		return
	}
	deadline := time.Now().Add(imageChipWait)
	for time.Now().Before(deadline) {
		rows, _, err := t.C.Screen(0)
		if err != nil || strings.Count(strings.Join(rows, "\n"), "[Image #") >= n {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// submitKey presses the harness submit key. Codex queues text with Tab while a
// turn runs; a background terminal alone is no turn.
func (t *Target) submitKey() error {
	working := false
	w, err := t.C.Watched()
	if err != nil {
		return err
	}
	if w != nil {
		working = w.Status.State == harness.Working && !w.Status.Background
	}
	return t.C.SendKeys(t.Typing.SubmitKey(working))
}

// Clear presses the harness clear key once.
func (t *Target) Clear() error {
	if err := t.cancelled(); err != nil {
		return err
	}
	return t.C.Batch(func() error { return t.C.SendKeys(t.Typing.ClearKey()) })
}

// SendEnter presses the submit key on its own.
func (t *Target) SendEnter() error {
	if err := t.cancelled(); err != nil {
		return err
	}
	return t.C.Batch(t.submitKey)
}

// WaitStart is the caller's start signal.
func (t *Target) WaitStart(timeout time.Duration) (bool, error) { return t.Start(timeout) }

// Screen is the plain screen rows joined by newlines.
func (t *Target) Screen() (string, error) {
	rows, _, err := t.C.Screen(0)
	return strings.Join(rows, "\n"), err
}

// Log is the logger the clear's pane capture goes to.
func (t *Target) Log() *slog.Logger { return t.IO.Log }

// ReadyToType runs the harness's ready-to-type step.
func (t *Target) ReadyToType() bool { return t.Typing.ReadyToType(t.IO) }

// InputEmpty runs the harness empty check when the holder serves styled rows.
func (t *Target) InputEmpty() (bool, bool, error) {
	state, _, err := InputRead(t.C, t.Typing)
	if err != nil {
		return false, false, err
	}
	return state == harness.InputEmpty, state == harness.InputEmpty || state == harness.InputNotEmpty, nil
}

// InputRead runs the harness empty check on a styled screen read. A holder
// that doesn't serve styled rows, or a read taken after a PTY resize and
// before the child's next output (untrusted), is InputUndecided; the
// ReadState's Resized flag tells the two apart.
func InputRead(c *holder.HolderClient, typing harness.Typing) (harness.InputState, holder.ReadState, error) {
	if !c.Supports(holder.OpScreenStyled) {
		return harness.InputUndecided, holder.ReadState{}, nil
	}
	rows, read, err := c.StyledScreenRead()
	if err != nil || read.Resized {
		return harness.InputUndecided, read, err
	}
	return typing.InputIsEmpty(rows), read, nil
}

// Input is what Deliver needs from a target: the Prompter's keys plus a type
// without the Enter, the image count and a screen read that says whether it
// can be trusted. *Target is the production one.
type Input interface {
	Prompter
	Type(text string) error
	ImageCount() int
	ReadRows() (rows []string, resized bool, err error)
}

// Policy is how Deliver delivers. A nil Slash is the turn-starting shape (Caps,
// Deadline, Now); otherwise the slash shape. PolicyFor picks one from the text.
type Policy struct {
	Caps  NudgeCaps
	Slash *SlashCaps
	// Launch, when set, retypes the text while the pane shows the unknown-command
	// marker, in either shape. Only a launch prompt sets it.
	Launch *LaunchCaps
	// Draft, when set, clears a person's draft before the text is typed.
	Draft    *DraftHooks
	Deadline time.Time
	// Now is the clock for Deadline; nil is time.Now.
	Now    func() time.Time
	Report func(claudeui.NudgeDecision)
}

// Deliver strips the characters claude would remove, then types text into t.
// The turn-starting shape nudges until the turn starts; the slash shape types
// once and presses the submit key again only while the box still holds text.
func Deliver(t Input, text string, policy Policy) error {
	if clean, n := Strip(text); n > 0 {
		logOf(t).Info("stripped invisible chars before typing", "count", n)
		text = clean
	}
	if d, ok := t.(DraftClearer); ok && policy.Draft != nil {
		if !readyToType(t) {
			return fmt.Errorf("%w: %w", claudeui.ErrStuckSubmission, claudeui.ErrNotReadyToType)
		}
		if err := d.ClearDraft(*policy.Draft); err != nil {
			return err
		}
	}
	if policy.Launch != nil {
		return deliverLaunch(t, text, policy)
	}
	if policy.Slash != nil {
		return deliverSlash(t, text, *policy.Slash, policy.Report, false)
	}
	return deliverTurn(t, text, policy)
}

func deliverTurn(t Input, text string, policy Policy) error {
	now := policy.Now
	if now == nil {
		now = time.Now
	}
	return PromptWithNudge(t, text, policy.Caps, policy.Deadline, now, policy.Report)
}
