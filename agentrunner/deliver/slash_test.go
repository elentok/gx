package deliver_test

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/deliver"
)

// read is one scripted screen read.
type read struct {
	rows    []string
	resized bool
}

// slashFake is a scripted Input. reads answers ReadRows in order (the last one
// repeats); the first read is the baseline. starts answers WaitStart in order
// (false once exhausted). Nothing sleeps: a wait only records its length.
type slashFake struct {
	reads     []read
	starts    []bool
	images    int
	enterErr  map[int]error // by the 1-based Enter count
	calls     []string
	waits     []time.Duration
	log       bytes.Buffer
	enters    int
	typedText string
}

func (f *slashFake) Submit(text string) error {
	f.calls = append(f.calls, "submit:"+text)
	return nil
}
func (f *slashFake) Clear() error { f.calls = append(f.calls, "clear"); return nil }
func (f *slashFake) Type(text string) error {
	f.calls = append(f.calls, "type")
	f.typedText = text
	return nil
}
func (f *slashFake) SendEnter() error {
	f.enters++
	f.calls = append(f.calls, "enter")
	return f.enterErr[f.enters]
}
func (f *slashFake) WaitStart(d time.Duration) (bool, error) {
	f.calls = append(f.calls, "wait")
	f.waits = append(f.waits, d)
	if len(f.starts) == 0 {
		return false, nil
	}
	s := f.starts[0]
	f.starts = f.starts[1:]
	return s, nil
}
func (f *slashFake) Screen() (string, error) { return "", nil }
func (f *slashFake) ImageCount() int         { return f.images }
func (f *slashFake) ReadRows() ([]string, bool, error) {
	f.calls = append(f.calls, "read")
	r := f.reads[0]
	if len(f.reads) > 1 {
		f.reads = f.reads[1:]
	}
	return r.rows, r.resized, nil
}
func (f *slashFake) Log() *slog.Logger {
	return slog.New(slog.NewTextHandler(&f.log, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

var rule = strings.Repeat("─", 40)

// box is a screen with claude's input box holding rows (the ❯ row first).
func box(rows ...string) []string {
	out := []string{"", rule, "❯ " + rows[0]}
	for _, r := range rows[1:] {
		out = append(out, "  "+r)
	}
	return append(out, rule, "  ? for shortcuts")
}

func ok(rows []string) read      { return read{rows: rows} }
func resized(rows []string) read { return read{rows: rows, resized: true} }

func (f *slashFake) deliver(text string) error {
	return deliver.Deliver(f, text, deliver.PolicyFor(text))
}

func TestSlash_Enters(t *testing.T) {
	menu := []string{"❯ /xyz", "  /xyz        runs it"}
	tests := []struct {
		name    string
		text    string
		reads   []read
		starts  []bool
		images  int
		enters  int
		wantErr error
	}{
		{"text in the box then gone: one extra Enter", "/xyz", []read{ok(box("/xyz")), ok(box("/xyz")), ok(box(""))}, nil, 0, 2, nil},
		{"swallowed twice: two extra Enters", "/xyz", []read{ok(box("/xyz")), ok(box("/xyz")), ok(box("/xyz")), ok(box(""))}, nil, 0, 3, nil},
		{"stuck box: three extra Enters then undecided", "/xyz", []read{ok(box("/xyz"))}, nil, 0, 4, deliver.ErrMaybeUnsent},
		{"a person's draft gets no Enter", "/xyz", []read{ok(box("/xyz")), ok(box("my own draft"))}, nil, 0, 1, nil},
		{"a placeholder gets no Enter", "/xyz", []read{ok(box("/xyz")), ok(box("Try \"fix lint errors\""))}, nil, 0, 1, nil},
		{"paste chip with equal M: Enter", "/xyz\na\nb", []read{ok(box("/xyz")), ok(box("[Pasted text #1 +2 lines]")), ok(box(""))}, nil, 0, 2, nil},
		{"paste chip with another M: none", "/xyz\na\nb", []read{ok(box("/xyz")), ok(box("[Pasted text #1 +9 lines]"))}, nil, 0, 1, nil},
		{"wrapped multi-row draft", "/xyz and a long argument that wraps", []read{ok(box("/xyz")), ok(box("/xyz and a long argume", "nt that wraps")), ok(box(""))}, nil, 0, 2, nil},
		{"dialog frame: echo row and one rule", "/model", []read{ok(box("/model")), ok([]string{"❯ /model", "", rule, "  Select model", "  ❯ 1. Haiku"})}, nil, 0, 1, nil},
		{"running compact: empty box", "/foo", []read{ok(box("/foo")), ok([]string{"❯ /foo", "✢ Compacting conversation… (1s)", rule, "❯ ", rule, "  esc to interrupt"})}, nil, 0, 1, nil},
		{"landed local command", "/model", []read{ok(box("/model")), ok([]string{"❯ /model", "  ⎿  Kept model as Haiku 5.5", rule, "❯ ", rule})}, nil, 0, 1, nil},
		{"resized read trusted after two polls", "/xyz", []read{ok(box("/xyz")), resized(box("/xyz")), resized(box("/xyz")), ok(box(""))}, nil, 0, 1, nil},
		{"torn frame with our text", "/xyz", []read{ok(box("/xyz")), ok([]string{rule, "/xyz", "  ? for shortcuts"})}, nil, 0, 1, deliver.ErrMaybeUnsent},
		{"no box, text only in the echo and menu rows", "/xyz", []read{ok(box("/xyz")), ok(menu)}, nil, 0, 1, nil},
		{"no box, text visible, screen unchanged", "/xyz", []read{ok([]string{"  /xyz here"}), ok([]string{"  /xyz here", "xyz happened /xyz"})}, nil, 0, 1, deliver.ErrMaybeUnsent},
		{"no box, text visible, screen changed", "/xyz", []read{ok([]string{"a"}), ok([]string{"b /xyz"})}, nil, 0, 1, deliver.ErrMaybeUnsent},
		{"no box, text gone, screen changed", "/xyz", []read{ok([]string{"a"}), ok([]string{"b"})}, nil, 0, 1, nil},
		{"no box, text gone, screen unchanged", "/xyz", []read{ok([]string{"a"})}, nil, 0, 1, nil},
		{"U+1BCA0 typed, box drew it without", "/xy\U0001BCA0z", []read{ok(box("/xy\U0001BCA0z")), ok(box("/xyz")), ok(box(""))}, nil, 0, 2, nil},
		{"a turn start ends the send at once", "/gx-skill", []read{ok(box("/gx-skill"))}, []bool{true}, 0, 1, nil},
		{"image chips before our text: Enter again", "/review", []read{ok(box("/review")), ok(box("[Image #1] [Image #2]  /review")), ok(box(""))}, nil, 2, 2, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &slashFake{reads: tt.reads, starts: tt.starts, images: tt.images}
			err := f.deliver(tt.text)
			if !errors.Is(err, tt.wantErr) || (tt.wantErr == nil && err != nil) {
				t.Fatalf("Deliver() error = %v, want %v", err, tt.wantErr)
			}
			if f.enters != tt.enters {
				t.Fatalf("Enters = %d, want %d (calls %v)", f.enters, tt.enters, f.calls)
			}
			if n := strings.Count(strings.Join(f.calls, " "), "type"); n != 1 {
				t.Fatalf("typed %d times, want once (calls %v)", n, f.calls)
			}
			if strings.Contains(strings.Join(f.calls, " "), "submit:") || strings.Contains(strings.Join(f.calls, " "), "clear") {
				t.Fatalf("slash shape retyped or cleared: %v", f.calls)
			}
		})
	}
}

func TestSlash_ResizePollsEvery25msUpToTheCap(t *testing.T) {
	f := &slashFake{reads: []read{ok(box("/xyz")), resized(box("/xyz"))}}
	err := f.deliver("/xyz")
	if !errors.Is(err, deliver.ErrMaybeUnsent) {
		t.Fatalf("Deliver() error = %v, want ErrMaybeUnsent", err)
	}
	if strings.Contains(err.Error(), "check the terminal") || !strings.Contains(err.Error(), "see the screen") {
		t.Fatalf("error = %q, want a reason that says to see the screen", err)
	}
	polls := 0
	for _, w := range f.waits[1:] {
		if w != 25*time.Millisecond {
			t.Fatalf("poll wait = %v, want 25ms (waits %v)", w, f.waits)
		}
		polls++
	}
	if polls != 40 {
		t.Fatalf("polls = %d, want 40", polls)
	}
	for _, want := range []string{"polls=40", "settle=1s", "visible=true"} {
		if !strings.Contains(f.log.String(), want) {
			t.Fatalf("log = %q, want %q", f.log.String(), want)
		}
	}
}

func TestSlash_UnknownCommandFailsAtOnceWithoutRetype(t *testing.T) {
	f := &slashFake{reads: []read{ok(box("/xyz")), ok(append(box("/xyz"), "Unknown command: /xyz"))}}
	err := f.deliver("/xyz")
	if !errors.Is(err, agentrunner.ErrUnknownSkill) || !errors.Is(err, agentrunner.ErrNotDelivered) {
		t.Fatalf("Deliver() error = %v, want ErrUnknownSkill wrapped in ErrNotDelivered", err)
	}
	if f.enters != 1 {
		t.Fatalf("Enters = %d, want 1", f.enters)
	}
}

func TestSlash_UnknownCommandAlreadyOnTheBaselineIsNotOurs(t *testing.T) {
	old := append(box("/xyz"), "Unknown command: /xyz")
	f := &slashFake{reads: []read{ok(old), ok(old), ok(box(""))}}
	if err := f.deliver("/xyz"); err != nil {
		t.Fatalf("Deliver() error = %v, want nil", err)
	}
}

func TestSlash_CodexShapeTypesOnceAndPressesOnce(t *testing.T) {
	f := &slashFake{reads: []read{ok(box("/xyz"))}}
	policy := deliver.PolicyFor("/xyz")
	policy.Slash.Settle = 0
	if err := deliver.Deliver(f, "/xyz", policy); err != nil {
		t.Fatalf("Deliver() error = %v", err)
	}
	if got := strings.Join(f.calls, " "); got != "type enter" {
		t.Fatalf("calls = %q, want %q", got, "type enter")
	}
}

func TestSlash_CancelStopsAllKeys(t *testing.T) {
	cancelled := errors.New("cancelled")
	f := &slashFake{reads: []read{ok(box("/xyz"))}, enterErr: map[int]error{2: cancelled}}
	if err := f.deliver("/xyz"); !errors.Is(err, cancelled) {
		t.Fatalf("Deliver() error = %v, want the cancel error", err)
	}
	if f.enters != 2 {
		t.Fatalf("Enters = %d, want 2: no key after the cancel", f.enters)
	}
}

func TestPolicyFor_PicksTheShapeFromTheText(t *testing.T) {
	tests := map[string]bool{
		"/model":          true,
		"/gx-implement a": true,
		"/compact":        false,
		"/compact foo":    false,
		"/compactx":       true,
		"​/cost":          true,
		"​/compact":       false,
		"hello":           false,
		"!ls":             false,
		"":                false,
	}
	for text, slash := range tests {
		if got := deliver.PolicyFor(text).Slash != nil; got != slash {
			t.Errorf("PolicyFor(%q) slash = %v, want %v", text, got, slash)
		}
	}
}

func TestSlash_CompactWithArgsUsesTheTurnStartingShape(t *testing.T) {
	f := &slashFake{starts: []bool{true}}
	if err := f.deliver("/compact foo"); err != nil {
		t.Fatalf("Deliver() error = %v", err)
	}
	if got := strings.Join(f.calls, " "); got != "submit:/compact foo wait" {
		t.Fatalf("calls = %q, want the turn-starting submit and wait", got)
	}
}
