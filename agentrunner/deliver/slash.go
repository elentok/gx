package deliver

import (
	"cmp"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/agentrunner/codexui"
	"github.com/elentok/gx/harness"
)

// ErrMaybeUnsent ends a slash send whose screen could not tell whether the
// command landed while it still shows gx's text. The command may or may not
// have run: callers must not resend.
var ErrMaybeUnsent = errors.New("deliver: the command may not have been sent")

// SlashCaps are the timings of the slash shape. A zero Settle is the Codex
// shape: type once, press the submit key once, read nothing.
type SlashCaps struct {
	// Settle is the wait for a turn start before the first read of the box.
	Settle time.Duration
	// Gap is the wait after each extra Enter before the next read.
	Gap time.Duration
	// ExtraEnters caps the Enters pressed after the first.
	ExtraEnters int
	// ResizePoll and ResizeCap bound the re-reads of a screen that was just
	// resized.
	ResizePoll, ResizeCap time.Duration
	// Marker is the pane text that says the agent does not know the command.
	// Empty is claude's "Unknown command: <our token>".
	Marker string
	// Watch, with a zero Settle, is how long the pane is watched for Marker
	// after the Enter, polling every WatchPoll (50ms when zero). Codex draws its
	// marker on screen only. Zero watches nothing.
	Watch, WatchPoll time.Duration
}

// DefaultSlashCaps are the slash timings.
var DefaultSlashCaps = SlashCaps{
	Settle:      time.Second,
	Gap:         time.Second,
	ExtraEnters: 3,
	ResizePoll:  25 * time.Millisecond,
	ResizeCap:   time.Second,
}

// SlashCapsFor are the slash timings for a session of kind. Codex sends once
// and watches the pane for its marker for watch; any other kind settles for
// settle, and its extra Enters wait gap.
func SlashCapsFor(kind agentrunner.Kind, settle, gap, watch time.Duration) SlashCaps {
	caps := DefaultSlashCaps
	caps.Settle, caps.Gap = settle, gap
	if kind == harness.Codex {
		caps.Settle = 0
		caps.Marker = codexui.UnknownCommandMarker
		caps.Watch = watch
	}
	return caps
}

// IsSlash reports whether text, after the hidden-character strip Deliver
// applies, is a "/" command that takes the slash shape: every "/" text except
// /compact with or without arguments, which starts a turn like any prompt.
func IsSlash(text string) bool {
	text, _ = Strip(text)
	return strings.HasPrefix(text, "/") && firstToken(text) != "/compact"
}

// PolicyFor picks the shape from the text, by IsSlash.
func PolicyFor(text string) Policy {
	p := Policy{Caps: DefaultNudgeCaps}
	if IsSlash(text) {
		caps := DefaultSlashCaps
		p.Slash = &caps
	}
	return p
}

func firstToken(text string) string {
	f := strings.Fields(text)
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

// ignorable are the characters dropped from both sides of the "starts with our
// text" comparison: whatever claude may strip or its box may not draw.
var ignorable = []*unicode.RangeTable{
	unicode.Cf, unicode.Other_Default_Ignorable_Code_Point, unicode.Variation_Selector, unicode.Cc,
}

func isIgnorable(r rune) bool {
	return r == 0x2028 || r == 0x2029 || unicode.In(r, ignorable...)
}

func dropIgnorable(s string) string {
	return strings.Map(func(r rune) rune {
		if isIgnorable(r) {
			return -1
		}
		return r
	}, s)
}

// ourPrefix is the first 20 characters of the first line of text, without the
// ignorable ones.
func ourPrefix(text string) string {
	first, _, _ := strings.Cut(text, "\n")
	if r := []rune(strings.TrimSpace(first)); len(r) > 20 {
		first = string(r[:20])
	}
	return strings.TrimSpace(dropIgnorable(first))
}

var pastedChip = regexp.MustCompile(`\[Pasted text[^\]]*\+(\d+) lines?\]`)

// boxText is the first box row after the ❯ marker with the leading image
// chips removed.
func boxText(box []string, chips int) string {
	row := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(box[0]), "❯"))
	for range chips {
		rest, ok := strings.CutPrefix(row, "[Image #")
		if !ok {
			break
		}
		_, after, _ := strings.Cut(rest, "]")
		row = strings.TrimSpace(after)
	}
	return row
}

// boxHoldsOurs reports whether the input box shows gx's own text: its start,
// or claude's chip for a paste of as many lines as text has. A draft that is
// neither may be a person's, which an extra Enter would submit.
func boxHoldsOurs(box []string, text string, chips int) bool {
	if len(box) == 0 {
		return false
	}
	row := boxText(box, chips)
	if m := pastedChip.FindStringSubmatch(row); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n == claudeui.PastedLines(text)
	}
	prefix := ourPrefix(text)
	return prefix != "" && strings.HasPrefix(strings.TrimSpace(dropIgnorable(row)), prefix)
}

// BoxHoldsText reports whether the input box on rows shows the start of text,
// or claude's chip for a paste of it. False when no box is drawn.
func BoxHoldsText(rows []string, text string, chips int) bool {
	box, ok := claudeui.InputBoxRows(rows)
	return ok && boxHoldsOurs(box, text, chips)
}

var menuRow = regexp.MustCompile(`^\s+/\S`)

// rowsHoldOurs reports whether the screen still shows gx's text: in the box
// row when a box exists, else on any row but the command's echo row and the
// suggestion menu's.
func rowsHoldOurs(rows []string, text string, chips int) bool {
	if box, ok := claudeui.InputBoxRows(rows); ok {
		return boxHoldsOurs(box, text, chips)
	}
	prefix := ourPrefix(text)
	if prefix == "" {
		return false
	}
	for _, row := range rows {
		if strings.HasPrefix(row, "❯") || menuRow.MatchString(row) {
			continue
		}
		if strings.Contains(dropIgnorable(row), prefix) {
			return true
		}
	}
	return false
}

func screenCount(rows []string, s string) int {
	n := 0
	for _, r := range rows {
		if strings.Contains(r, s) {
			n++
		}
	}
	return n
}

func screenHas(rows []string, s string) bool {
	for _, r := range rows {
		if strings.Contains(r, s) {
			return true
		}
	}
	return false
}

func logOf(t Input) *slog.Logger {
	if lp, ok := t.(LogProvider); ok && lp.Log() != nil {
		return lp.Log()
	}
	return slog.Default()
}

// slashSend is one slash delivery: its target, text and what the screen showed
// before the first Enter.
type slashSend struct {
	t        Input
	text     string
	chips    int
	caps     SlashCaps
	baseline []string
	launch   bool
}

// unknown fails the send when the pane shows the unknown-command marker
// (claude's with our token) that the baseline did not. A launch send never
// fails here: its retype window decides.
func (s *slashSend) unknown(rows []string) error {
	if s.launch {
		return nil
	}
	marker := cmp.Or(s.caps.Marker, claudeui.UnknownCommandMarker+" "+firstToken(s.text))
	// A count, not presence: a resend of a failed command finds the first
	// failure's marker already on screen.
	if screenCount(rows, marker) <= screenCount(s.baseline, marker) {
		return nil
	}
	return errUnknown(s.text, marker)
}

func errUnknown(text, marker string) error {
	return &agentrunner.UnknownCommandError{Text: text, Marker: marker}
}

// watch reads the pane for Marker for caps.Watch after the Enter.
func (s *slashSend) watch() error {
	poll := cmp.Or(s.caps.WatchPoll, 50*time.Millisecond)
	for waited := time.Duration(0); s.caps.Watch > 0; waited += poll {
		rows, _, err := s.t.ReadRows()
		if err == nil {
			if err := s.unknown(rows); err != nil {
				return err
			}
		}
		if waited >= s.caps.Watch {
			break
		}
		if started, err := s.t.WaitStart(poll); err != nil || started {
			return err
		}
	}
	return nil
}

// settled is one trusted-or-not read of the screen.
type settled struct {
	rows    []string
	trusted bool
	polls   int
}

// read polls the screen every ResizePoll until a read is not resized and shows
// a box, or ResizeCap passes. The polls wait on the start signal, so a turn
// that starts meanwhile ends the send.
func (s *slashSend) read() (r settled, started bool, err error) {
	for waited := time.Duration(0); ; {
		rows, resized, err := s.t.ReadRows()
		if err != nil {
			return r, false, fmt.Errorf("reading the screen after a slash command: %w", err)
		}
		r.rows = rows
		if err := s.unknown(rows); err != nil {
			return r, false, err
		}
		if !resized && claudeui.InputBox(rows) {
			r.trusted = true
			return r, false, nil
		}
		if waited >= s.caps.ResizeCap {
			return r, false, nil
		}
		started, err = s.t.WaitStart(s.caps.ResizePoll)
		waited += s.caps.ResizePoll
		r.polls++
		if err != nil || started {
			return r, started, err
		}
	}
}

func (s *slashSend) undecided(polls int, visible bool) error {
	logOf(s.t).Info("slash command undecided", "polls", polls, "settle", s.caps.Settle, "visible", visible)
	return fmt.Errorf("%w: see the screen to tell whether %q ran (%d re-reads)", ErrMaybeUnsent, firstToken(s.text), polls)
}

// typeSubmitter is an Input that types and presses the first Enter under one
// lock and batch, so another send's keys or draft clear cannot land between
// them. between runs after the text is typed and before the Enter.
type typeSubmitter interface {
	TypeThenSubmit(text string, between func()) error
}

// deliverSlash types the command once and presses the submit key once. After
// the settle it presses again only on a trusted read whose box row still shows
// gx's text. It never retypes.
func deliverSlash(t Input, text string, caps SlashCaps, report func(claudeui.NudgeDecision), launch bool) error {
	if !readyToType(t) {
		return fmt.Errorf("%w: %w", claudeui.ErrStuckSubmission, claudeui.ErrNotReadyToType)
	}
	s := &slashSend{t: t, text: text, chips: t.ImageCount(), caps: caps, launch: launch}
	readBaseline := func() {
		if caps.Settle > 0 || caps.Watch > 0 {
			s.baseline, _, _ = t.ReadRows()
		}
	}
	if ts, ok := t.(typeSubmitter); ok {
		if err := ts.TypeThenSubmit(text, readBaseline); err != nil {
			return err
		}
	} else {
		if err := t.Type(text); err != nil {
			return err
		}
		readBaseline()
		if err := t.SendEnter(); err != nil {
			return err
		}
	}
	if caps.Settle <= 0 {
		return s.watch()
	}
	if started, err := t.WaitStart(caps.Settle); err != nil || started {
		return err
	}
	for extra := 0; ; extra++ {
		r, started, err := s.read()
		if err != nil || started {
			return err
		}
		visible := rowsHoldOurs(r.rows, text, s.chips)
		switch {
		case !visible:
			return nil
		case !r.trusted || extra >= caps.ExtraEnters:
			return s.undecided(r.polls, visible)
		}
		if report != nil {
			report(claudeui.NudgeEnter)
		}
		if err := t.SendEnter(); err != nil {
			return err
		}
		if started, err := t.WaitStart(caps.Gap); err != nil || started {
			return err
		}
	}
}
