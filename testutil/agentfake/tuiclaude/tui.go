package tuiclaude

import . "github.com/elentok/gx/testutil/agentfake"

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/elentok/gx/testutil/agentfake/triageplay"
)

type tui struct {
	out io.Writer
	// ack gets one byte after each handled control.
	ack     io.Writer
	opts    tuiOptions
	started time.Time
	// modelErrored is set once the first message got the model's API error.
	modelErrored bool
	// limitResetsAt, when set (Unix seconds), replaces the five-hour window's reset in the
	// usage the statusLine reports, so a rate limit's reset is relative to when it was hit.
	limitResetsAt int64
	// limitLifted makes the statusLine report the five-hour window as unused once the
	// rate limit that set limitResetsAt has been resumed past.
	limitLifted bool
	// notice is a line drawn under the input, such as an unknown-command error.
	notice string
	// noticeRows is how many times notice is drawn: an unknown command repeated
	// adds a line, as claude's transcript does.
	noticeRows int
	// vim is set by ActionVimToggle: the input then draws the INSERT footer.
	vim        bool
	pidPath    string
	ctlPath    string
	transcript string
	pid        PidFile
	stalled    bool
	// fast makes the next turn end as it starts.
	fast bool
	// dialogNext makes the next "/" command open a dialog.
	dialogNext bool
	// busyNext makes the next "/" command start a turn that stays busy until a
	// control ends it.
	busyNext bool
	// commandOutput is the stdout the next "/" command writes, set by
	// ActionCommandOutput.
	commandOutput *string
	// ignoreAllow makes Enter leave a dialog open.
	ignoreAllow bool
	// quit ends the fake cleanly after the current input, as /exit does.
	quit bool
	// dialog replaces the default waiting rows while set by ActionBlock.
	dialog []string
	// trustYes is whether the trust dialog's cursor is on "Yes"; arrow
	// collects an arrow key's escape sequence.
	trustYes bool
	arrow    []byte
	typed    strings.Builder
	prompt   string
	// seq collects an escape sequence in mouse mode; wheelUp and wheelDown
	// count the wheel events read.
	seq                []byte
	wheelUp, wheelDown int
	design             DesignState
	// triageProject is the project of the Triage session this fake runs; a
	// plain typed message then goes to the Triage player.
	triageProject string
	// startDesign is the card of the Start Design chat this fake runs; a plain
	// typed message then edits or approves it.
	startDesign *triageplay.DesignCard
	// resume is the ticket a ratelimit or question-menu worker finishes on the
	// next prompt, whatever that prompt says.
	resume *PendingWork
	// outageWork is the ticket a plain prompt works once an outage has passed.
	outageWork *PendingWork
	// images are the paths behind the [Image #N] chips in the input.
	images []string
	// pastes are the real texts behind the [Pasted text #N] chips in the input.
	pastes []string
	paste  pasteState
	// menuClosed is set by Esc on an open suggestion menu, until the next typed key.
	menuClosed bool
	// swallow is how many more Enters do nothing (ActionSwallowEnter).
	swallow int
	// invisibleShown is set while notice is the "Removed N invisible character" line.
	invisibleShown bool
	// descendants is the process tree an ActionDescendants started.
	descendants DescendantTree
	// askHook is the AskUserQuestion hook an ActionAskQuestion started; Esc ends it.
	askHook *exec.Cmd
	// askUI is the AskUserQuestion dialog an ActionAskScreen opened; it takes
	// every key until it ends.
	askUI *askUI
	// localDialog is the /model or /help dialog that is open ("" for none); it
	// takes every key until it closes.
	localDialog string
	// compact is the running /compact, nil when none runs.
	compact *compactRun
	// landed are the rows of a finished local command, drawn above the box
	// until the next submitted prompt.
	landed []string
	// undecidedNext arms the undecided mode for the next "/" draft; undecided
	// is it active (see ActionUndecided).
	undecidedNext, undecided bool
}

func (t *tui) run(in io.Reader, ctl io.Reader) (int, error) {
	events := TuiEvents(in, ctl)
	// A delayed redraw is posted back to the loop as a resize event so the loop
	// stays the only writer to the PTY.
	redraws := make(chan TuiEvent)
	redraw := t.draw
	if t.opts.redrawDelay > 0 {
		redraw = func() {
			time.AfterFunc(t.opts.redrawDelay, func() { redraws <- TuiEvent{Resized: true} })
		}
	}
	if t.opts.mouse {
		fmt.Fprint(t.out, "\x1b[?1049h\x1b[?1000h\x1b[?1006h")
	}
	fmt.Fprint(t.out, BracketedPasteOn)
	if t.opts.trust {
		t.draw()
	} else if err := t.startSession(); err != nil {
		return 0, err
	}
	for {
		var ev TuiEvent
		select {
		case e, ok := <-events:
			if !ok {
				return 0, nil
			}
			ev = e
		case ev = <-redraws:
		}
		switch {
		case ev.Err != nil:
			// The PTY closed: claude exits cleanly.
			if PtyClosed(ev.Err) {
				return 0, t.exit(0)
			}
			return 0, ev.Err
		case ev.Resized:
			t.draw()
		case ev.Control != nil:
			if ev.Control.Action == ActionExit {
				return ev.Control.Code, t.exit(ev.Control.Code)
			}
			if err := t.handleControl(*ev.Control); err != nil {
				return 0, err
			}
			if _, err := t.ack.Write([]byte{'.'}); err != nil {
				return 0, err
			}
		case t.askUI != nil:
			if err := t.askScreenKey(ev.Key); err != nil {
				return 0, err
			}
		case t.localDialog != "":
			if err := t.localDialogKey(ev.Key); err != nil {
				return 0, err
			}
		case t.opts.trust:
			if quit, err := t.trustKey(ev.Key); quit || err != nil {
				return 1, err
			}
		case t.opts.wedged:
		case t.pasteKey(ev.Key):
		case t.opts.mouse && (ev.Key == EscKey || len(t.seq) > 0):
			if err := t.mouseSeqKey(ev.Key); err != nil {
				return 0, err
			}
		case ev.Key == '\r' && t.opts.hung:
		case ev.Key == '\r':
			// A raw LF is a line break in the input, as in claude, not Enter.
			err := t.submit()
			// The box is empty after Enter; a path that starts no turn draws nothing,
			// and the screen would keep showing the sent text.
			t.draw()
			if err != nil {
				return 0, err
			}
			if t.quit {
				return 0, t.exit(0)
			}
		case ev.Key == EscKey:
			if err := t.interrupt(); err != nil {
				return 0, err
			}
		case ev.Key == CtrlC:
			if err := t.cancel(); err != nil {
				return 0, err
			}
		case ev.Key == CtrlU && t.opts.clearIgnored:
		case ev.Key == CtrlU:
			t.typed.Reset()
			if t.opts.clearLeavesI {
				t.opts.clearLeavesI = false
				t.typed.WriteByte('i')
			}
			redraw()
		case t.opts.dropTyped:
		default:
			t.typed.WriteByte(ev.Key)
			t.menuClosed = false
			if t.invisibleShown {
				t.notice, t.invisibleShown, t.noticeRows = "", false, 0
			}
			// Echoed as claude does, the terminal's cursor after it.
			redraw()
		}
	}
}

// mouseSeqKey collects an escape sequence: an SGR mouse report counts its
// wheel turns; anything else is an Esc, and interrupts.
func (t *tui) mouseSeqKey(key byte) error {
	t.seq = append(t.seq, key)
	s := string(t.seq)
	if s == "\x1b" || s == "\x1b[" || (strings.HasPrefix(s, "\x1b[<") && key != 'M' && key != 'm') {
		return nil
	}
	t.seq = nil
	if !strings.HasPrefix(s, "\x1b[<") {
		return t.interrupt()
	}
	switch button, _, _ := strings.Cut(s[len("\x1b[<"):], ";"); button {
	case "64":
		t.wheelUp++
	case "65":
		t.wheelDown++
	}
	t.draw()
	return nil
}

// trustKey takes one key at the trust dialog and reports whether it picked
// "No, exit". Arrow keys toggle between the two options.
func (t *tui) trustKey(key byte) (bool, error) {
	if t.opts.dropKey {
		t.opts.dropKey = false
		return false, nil
	}
	if key == EscKey || len(t.arrow) > 0 {
		t.arrow = append(t.arrow, key)
		if len(t.arrow) == len("\x1b[B") {
			if s := string(t.arrow); s == "\x1b[A" || s == "\x1b[B" {
				t.trustYes = !t.trustYes
				t.draw()
			}
			t.arrow = nil
		}
		return false, nil
	}
	if key != '\r' && key != '\n' {
		return false, nil
	}
	if !t.trustYes {
		return true, nil
	}
	return false, t.startSession()
}

// startSession writes the pid file, first without a status, as claude does
// while it loads, then idle.
func (t *tui) startSession() error {
	t.opts.trust = false
	if err := t.writePid(); err != nil {
		return err
	}
	// Like real claude, the transcript appears with the first user record.
	return t.setStatus(StatusIdle, "")
}
