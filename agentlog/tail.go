package agentlog

import (
	"bufio"
	"io"
	"os"
	"strings"
)

// Tail follows a Log without blocking: each Step reads what was added since
// the last one. The CLI sleeps between steps; the TUI steps once per tick.
type Tail struct {
	log     Log
	f       *os.File
	r       *bufio.Reader
	render  func([]byte) []string
	partial string
	ended   bool
}

// OpenTail opens log at its offset. A nil render means the log's own, else Render.
func OpenTail(log Log, render func([]byte) []string) (*Tail, error) {
	f, err := os.Open(log.Path)
	if err != nil {
		return nil, err
	}
	if _, err := f.Seek(log.Offset, io.SeekStart); err != nil {
		f.Close()
		return nil, err
	}
	if render == nil {
		render = log.Render
	}
	if render == nil {
		render = Render
	}
	return &Tail{log: log, f: f, r: bufio.NewReader(f), render: render}, nil
}

// Step returns the rendered lines completed since the last step, holding a
// trailing partial line back. Once the agent has ended it reads one last time,
// to catch what was written just before exit, flushes the partial line and
// reports ended.
func (t *Tail) Step() (lines []string, ended bool, err error) {
	if t.ended {
		return nil, true, nil
	}
	lines, err = t.readToEnd()
	if err != nil || t.log.Ended == nil || !t.log.Ended() {
		return lines, false, err
	}
	more, err := t.readToEnd()
	if err != nil {
		return nil, false, err
	}
	t.ended = true
	return append(append(lines, more...), t.Flush()...), true, nil
}

// Flush renders and drops the held partial line, for a reader that stops
// before the line is complete.
func (t *Tail) Flush() []string {
	lines := t.format(t.partial)
	t.partial = ""
	return lines
}

func (t *Tail) Close() error { return t.f.Close() }

func (t *Tail) readToEnd() ([]string, error) {
	var lines []string
	for {
		chunk, err := t.r.ReadString('\n')
		if err == io.EOF {
			t.partial += chunk
			return lines, nil
		}
		if err != nil {
			return nil, err
		}
		lines = append(lines, t.format(t.partial+strings.TrimSuffix(chunk, "\n"))...)
		t.partial = ""
	}
}

func (t *Tail) format(line string) []string {
	if line == "" {
		return nil
	}
	return t.render([]byte(line))
}
