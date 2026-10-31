package agentfake

import "time"

const (
	tabKey = 0x09
	// codexBackgroundTitle is the title Codex shows with only a background
	// terminal running.
	codexBackgroundTitle = "Ready"
	// codexEscWait is how long a lone Esc waits for the rest of an escape
	// sequence before it counts as the Esc key.
	codexEscWait = 40 * time.Millisecond
	// actionInterrupt is the Esc timer's own control; a test sends Ctrl+C or Esc
	// as keys instead.
	actionInterrupt = "interrupt"

	pasteStart = "\x1b[200~"
	pasteEnd   = "\x1b[201~"
)

// codexInput is the keys-to-box layer of the Codex fake: bracketed paste keeps
// its newlines, Tab queues text while a turn runs, and Ctrl+C or Esc ends the
// turn.
type codexInput struct {
	// seq collects an escape sequence; nil when none is open.
	seq     []byte
	pasting bool
	// afterCR drops the LF of a pasted CRLF.
	afterCR bool
	// escTimer fires actionInterrupt for an Esc nothing followed.
	escTimer *time.Timer
	// queue holds the texts Tab queued while a turn ran, oldest first.
	queue []string
}

// key takes one typed byte.
func (c *codexFake) key(b byte) error {
	in := &c.input
	if in.seq != nil {
		return c.sequenceKey(b)
	}
	if b == escKey {
		in.seq = []byte{b}
		in.escTimer = time.AfterFunc(codexEscWait, func() {
			_ = SendControl(c.ctlPath, Control{Action: actionInterrupt})
		})
		return nil
	}
	if in.pasting {
		c.pasteKey(b)
		return nil
	}
	switch b {
	case ctrlC:
		return c.interrupt()
	case tabKey:
		if c.working && c.box.text() != "" {
			in.queue = append(in.queue, c.box.text())
			c.box.lines = []string{""}
			c.box.draw()
		}
		return nil
	}
	text, submitted := c.box.key(b)
	// Real Codex queues a prompt typed mid-turn with Tab; Enter alone drops it.
	if submitted && text != "" && !c.working {
		return c.submitLogged(text)
	}
	return nil
}

// sequenceKey continues an escape sequence. A byte after a lone Esc that is
// not '[' makes the Esc a key of its own.
func (c *codexFake) sequenceKey(b byte) error {
	in := &c.input
	if in.escTimer != nil {
		in.escTimer.Stop()
		in.escTimer = nil
	}
	if len(in.seq) == 1 && b != '[' {
		in.seq = nil
		if err := c.interrupt(); err != nil {
			return err
		}
		return c.key(b)
	}
	in.seq = append(in.seq, b)
	if len(in.seq) == 2 || b < 0x40 || b > 0x7e {
		return nil
	}
	switch string(in.seq) {
	case pasteStart:
		in.pasting = true
	case pasteEnd:
		in.pasting = false
	}
	in.seq = nil
	return nil
}

// pasteKey adds a pasted byte to the box; every line break is kept.
func (c *codexFake) pasteKey(b byte) {
	in := &c.input
	if b == '\n' && in.afterCR {
		in.afterCR = false
		return
	}
	in.afterCR = b == '\r'
	if b == '\r' {
		b = '\n'
	}
	c.box.key(b)
}

// submitLogged logs a submitted entry as the Claude fake does, then starts it.
func (c *codexFake) submitLogged(text string) error {
	if err := logSubmitted(c.id, text); err != nil {
		return err
	}
	return c.submit(text)
}

// interrupt is Ctrl+C or Esc: it ends a running turn, leaving a background
// terminal and its Ready title in place.
func (c *codexFake) interrupt() error {
	if err := logSubmitted(c.id, ProgramInterruptEntry); err != nil {
		return err
	}
	if !c.working {
		return nil
	}
	c.working = false
	c.showResting()
	return nil
}

// showResting draws what Codex shows between turns: working with the Ready
// title while a background terminal runs, idle otherwise.
func (c *codexFake) showResting() {
	if c.background {
		c.setTitle(codexBackgroundTitle)
		c.report("working")
		return
	}
	c.setTitle(c.title)
	c.report("idle")
}

// runQueued submits the oldest text Tab queued, once a turn has ended.
func (c *codexFake) runQueued() error {
	in := &c.input
	if c.working || len(in.queue) == 0 {
		return nil
	}
	text := in.queue[0]
	in.queue = in.queue[1:]
	return c.submitLogged(text)
}

func (c *codexFake) backgroundControl(running bool) {
	c.background = running
	if !c.working {
		c.showResting()
	}
}
