package agentfake

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type tui struct {
	out io.Writer
	// ack gets one byte after each handled control.
	ack     io.Writer
	opts    tuiOptions
	started time.Time
	// limitResetsAt, when set (Unix seconds), replaces the five-hour window's reset in the
	// usage the statusLine reports, so a rate limit's reset is relative to when it was hit.
	limitResetsAt int64
	// notice is a line drawn under the input, such as an unknown-command error.
	notice string
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
	// resume is the ticket a ratelimit or question-menu worker finishes on the
	// next prompt, whatever that prompt says.
	resume *PendingWork
}

type PendingWork struct {
	Cfg     WorkerConfig
	Address string
}

// DesignState is set once a /gx-design prompt arrives; rounds counts the ones
// played.
type DesignState struct {
	Active bool
	rounds int
	Turns  int
	// script is the turn script of the map this session designs, from
	// WorkerConfig.DesignByMap; nil falls back to WorkerConfig.Design.
	script []string
}

// TuiEvent is one key from the terminal, one control, or the end of input.
type TuiEvent struct {
	Key     byte
	Control *Control
	Resized bool
	Err     error
}

func (t *tui) run(in io.Reader, ctl io.Reader) (int, error) {
	events := TuiEvents(in, ctl)
	if t.opts.mouse {
		fmt.Fprint(t.out, "\x1b[?1049h\x1b[?1000h\x1b[?1006h")
	}
	if t.opts.trust {
		t.draw()
	} else if err := t.startSession(); err != nil {
		return 0, err
	}
	for ev := range events {
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
		case t.opts.trust:
			if quit, err := t.trustKey(ev.Key); quit || err != nil {
				return 1, err
			}
		case t.opts.mouse && (ev.Key == EscKey || len(t.seq) > 0):
			if err := t.mouseSeqKey(ev.Key); err != nil {
				return 0, err
			}
		case ev.Key == '\r':
			// A raw LF is a line break in the input, as in claude, not Enter.
			if err := t.submit(); err != nil {
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
		case ev.Key == ctrlU:
			t.typed.Reset()
			if t.opts.clearLeavesI {
				t.opts.clearLeavesI = false
				t.typed.WriteByte('i')
			}
			t.draw()
		default:
			t.typed.WriteByte(ev.Key)
			// Echoed as claude does, the terminal's cursor after it.
			t.draw()
		}
	}
	return 0, nil
}

func PtyClosed(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, syscall.EIO)
}

// TuiEvents merges the keys read from in with the controls read from ctl.
func TuiEvents(in io.Reader, ctl io.Reader) <-chan TuiEvent {
	events := make(chan TuiEvent)
	go func() {
		r := bufio.NewReader(in)
		for {
			b, err := r.ReadByte()
			if err != nil {
				events <- TuiEvent{Err: err}
				return
			}
			events <- TuiEvent{Key: b}
		}
	}()
	go func() {
		scanner := bufio.NewScanner(ctl)
		for scanner.Scan() {
			var c Control
			if err := json.Unmarshal(scanner.Bytes(), &c); err != nil {
				events <- TuiEvent{Err: fmt.Errorf("bad control line %q: %w", scanner.Text(), err)}
				return
			}
			events <- TuiEvent{Control: &c}
		}
	}()
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	go func() {
		for range winch {
			events <- TuiEvent{Resized: true}
		}
	}()
	return events
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
	if err := t.setStatus(StatusIdle, ""); err != nil {
		return err
	}
	// Claude has its transcript file once it is ready.
	if !t.opts.readySignal {
		return nil
	}
	if t.opts.readyDelay > 0 {
		time.AfterFunc(t.opts.readyDelay, func() {
			_ = t.touchTranscript()
			t.draw()
		})
		return nil
	}
	return t.touchTranscript()
}

func (t *tui) touchTranscript() error {
	path, err := t.transcriptPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}

// ready is whether the input box shows yet.
func (t *tui) ready() bool { return time.Since(t.started) >= t.opts.readyDelay }
