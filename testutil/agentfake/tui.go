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
	design             designState
	// resume is the ticket a ratelimit or question-menu worker finishes on the
	// next prompt, whatever that prompt says.
	resume *pendingWork
}

type pendingWork struct {
	cfg     WorkerConfig
	address string
}

// designState is set once a /gx-design prompt arrives; rounds counts the ones
// played.
type designState struct {
	active bool
	rounds int
	turns  int
	// script is the turn script of the map this session designs, from
	// WorkerConfig.DesignByMap; nil falls back to WorkerConfig.Design.
	script []string
}

// tuiEvent is one key from the terminal, one control, or the end of input.
type tuiEvent struct {
	key     byte
	control *Control
	resized bool
	err     error
}

func (t *tui) run(in io.Reader, ctl io.Reader) (int, error) {
	events := tuiEvents(in, ctl)
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
		case ev.err != nil:
			// The PTY closed: claude exits cleanly.
			if ptyClosed(ev.err) {
				return 0, t.exit(0)
			}
			return 0, ev.err
		case ev.resized:
			t.draw()
		case ev.control != nil:
			if ev.control.Action == ActionExit {
				return ev.control.Code, t.exit(ev.control.Code)
			}
			if err := t.handleControl(*ev.control); err != nil {
				return 0, err
			}
			if _, err := t.ack.Write([]byte{'.'}); err != nil {
				return 0, err
			}
		case t.opts.trust:
			if quit, err := t.trustKey(ev.key); quit || err != nil {
				return 1, err
			}
		case t.opts.mouse && (ev.key == escKey || len(t.seq) > 0):
			if err := t.mouseSeqKey(ev.key); err != nil {
				return 0, err
			}
		case ev.key == '\r':
			// A raw LF is a line break in the input, as in claude, not Enter.
			if err := t.submit(); err != nil {
				return 0, err
			}
			if t.quit {
				return 0, t.exit(0)
			}
		case ev.key == escKey:
			if err := t.interrupt(); err != nil {
				return 0, err
			}
		case ev.key == ctrlC:
			if err := t.cancel(); err != nil {
				return 0, err
			}
		case ev.key == ctrlU:
			t.typed.Reset()
			if t.opts.clearLeavesI {
				t.opts.clearLeavesI = false
				t.typed.WriteByte('i')
			}
			t.draw()
		default:
			t.typed.WriteByte(ev.key)
			// Echoed as claude does, the terminal's cursor after it.
			t.draw()
		}
	}
	return 0, nil
}

func ptyClosed(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, syscall.EIO)
}

// tuiEvents merges the keys read from in with the controls read from ctl.
func tuiEvents(in io.Reader, ctl io.Reader) <-chan tuiEvent {
	events := make(chan tuiEvent)
	go func() {
		r := bufio.NewReader(in)
		for {
			b, err := r.ReadByte()
			if err != nil {
				events <- tuiEvent{err: err}
				return
			}
			events <- tuiEvent{key: b}
		}
	}()
	go func() {
		scanner := bufio.NewScanner(ctl)
		for scanner.Scan() {
			var c Control
			if err := json.Unmarshal(scanner.Bytes(), &c); err != nil {
				events <- tuiEvent{err: fmt.Errorf("bad control line %q: %w", scanner.Text(), err)}
				return
			}
			events <- tuiEvent{control: &c}
		}
	}()
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	go func() {
		for range winch {
			events <- tuiEvent{resized: true}
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
	if key == escKey || len(t.arrow) > 0 {
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
