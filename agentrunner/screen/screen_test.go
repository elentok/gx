package screen_test

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/agentrunner/screen"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// replays are claude's raw PTY output recorded by the phase 2 map (evidence
// 02-vt), cut just before exit. They gate every x/vt pin bump: a bump that
// changes what gx reads off claude's screen fails here.
var replays = []struct {
	name       string
	cols, rows int
	anchors    []string
	title      string
}{
	{
		name: "w60", cols: 60, rows: 30,
		anchors: []string{"Early towers used open fires"},
		title:   "✳ Lighthouse paragraph writing request",
	},
	{
		name: "w250", cols: 250, rows: 30,
		anchors: []string{
			"Lighthouses have guided sailors along dangerous coastlines",
			"while modern beacons use electric lights",
		},
		title: "✳ Lighthouses paragraph writing request",
	},
	{
		name: "w60-default", cols: 60, rows: 20,
		anchors: []string{"Their bright beams"},
		// claude clears the title on the way out of the main screen.
		title: "",
	},
}

var ws = regexp.MustCompile(`\s+`)

func replay(t *testing.T, name string, cols, rows int) (*screen.Screen, []byte) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name+".raw"))
	if err != nil {
		t.Fatal(err)
	}
	s := screen.NewScreen(cols, rows)
	t.Cleanup(func() { _ = s.Close() })
	if _, err := s.Write(raw); err != nil {
		t.Fatal(err)
	}
	return s, raw
}

func TestScreen_ReplayGolden(t *testing.T) {
	for _, r := range replays {
		t.Run(r.name, func(t *testing.T) {
			s, _ := replay(t, r.name, r.cols, r.rows)
			scrollback, rows := s.ScrollbackTail(1000), s.Rows()
			got := "-- scrollback\n" + strings.Join(scrollback, "\n") + "\n-- screen\n" + strings.Join(rows, "\n") + "\n"

			golden := filepath.Join("testdata", r.name+".golden")
			if *update {
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Errorf("replay differs from %s (rerun with -update after checking the diff)\ngot:\n%s", golden, got)
			}

			joined := ws.ReplaceAllString(strings.Join(append(scrollback, rows...), " "), " ")
			for _, a := range r.anchors {
				if !strings.Contains(joined, a) {
					t.Errorf("anchor %q not on screen or in scrollback", a)
				}
			}
		})
	}
}

func TestScreen_ScrollbackTailIsLastN(t *testing.T) {
	s := screen.NewScreen(10, 2)
	t.Cleanup(func() { _ = s.Close() })
	_, _ = s.Write([]byte("a\r\nb\r\nc\r\nd\r\ne"))
	if got := strings.Join(s.ScrollbackTail(2), ","); got != "b,c" {
		t.Errorf("ScrollbackTail(2) = %q, want b,c", got)
	}
	if got := strings.Join(s.Rows(), ","); got != "d,e" {
		t.Errorf("Rows() = %q, want d,e", got)
	}
}

// Run under -race: the drain goroutine must not touch the emulator after
// Close returns.
func TestScreen_CloseWhileDrainingIsRaceFree(t *testing.T) {
	s := screen.NewScreen(10, 2)
	// Cursor-position and device-attribute queries make the emulator reply.
	_, _ = s.Write([]byte("\x1b[6n\x1b[c"))
	if err := s.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second Close() = %v", err)
	}
}

func TestScreen_Resize(t *testing.T) {
	s := screen.NewScreen(10, 2)
	t.Cleanup(func() { _ = s.Close() })
	s.Resize(20, 3)
	_, _ = s.Write([]byte("0123456789abcdefghij"))
	if got := s.Rows()[0]; got != "0123456789abcdefghij" {
		t.Errorf("row 0 after resize = %q", got)
	}
}

var sgr = regexp.MustCompile(`\x1b\[[0-9;:]*m`)

func TestScreen_StyledRowsKeepColoursOfReplay(t *testing.T) {
	for _, r := range replays {
		t.Run(r.name, func(t *testing.T) {
			s, _ := replay(t, r.name, r.cols, r.rows)
			styled := s.StyledRows()
			if len(styled) != r.rows {
				t.Fatalf("StyledRows() has %d rows, want %d", len(styled), r.rows)
			}
			if !sgr.MatchString(strings.Join(styled, "\n")) {
				t.Error("StyledRows() carries no SGR sequences")
			}
			plain := s.Rows()
			for i, row := range styled {
				got := strings.TrimRight(sgr.ReplaceAllString(row, ""), " ")
				want := ""
				if i < len(plain) {
					want = plain[i]
				}
				if got != want {
					t.Errorf("row %d without styles = %q, want %q", i, got, want)
				}
			}
		})
	}
}

func TestScreen_StyledRowColour(t *testing.T) {
	s := screen.NewScreen(10, 2)
	t.Cleanup(func() { _ = s.Close() })
	_, _ = s.Write([]byte("\x1b[31mred\x1b[0m"))
	if got := s.StyledRows()[0]; !strings.Contains(got, "\x1b[31mred") {
		t.Errorf("StyledRows()[0] = %q, want red SGR", got)
	}
}

func TestScreen_Cursor(t *testing.T) {
	s := screen.NewScreen(10, 4)
	t.Cleanup(func() { _ = s.Close() })
	_, _ = s.Write([]byte("ab\r\ncde"))
	if got, want := s.Cursor(), (screen.Cursor{X: 3, Y: 1, Visible: true}); got != want {
		t.Errorf("Cursor() = %+v, want %+v", got, want)
	}
	_, _ = s.Write([]byte("\x1b[?25l"))
	if s.Cursor().Visible {
		t.Error("Cursor().Visible after DECTCEM reset = true")
	}
	_, _ = s.Write([]byte("\x1b[?25h"))
	if !s.Cursor().Visible {
		t.Error("Cursor().Visible after DECTCEM set = false")
	}
}

func TestScreen_DirtyRowsListOnlyChangedRows(t *testing.T) {
	s := screen.NewScreen(10, 4)
	t.Cleanup(func() { _ = s.Close() })
	_, _ = s.Write([]byte("a\r\nb\r\nc"))
	s.ClearDirty()
	if got := s.DirtyRows(); len(got) != 0 {
		t.Errorf("DirtyRows() after ClearDirty = %v, want none", got)
	}
	_, _ = s.Write([]byte("\x1b[2;1Hx"))
	if got := s.DirtyRows(); !slices.Equal(got, []int{1}) {
		t.Errorf("DirtyRows() = %v, want [1]", got)
	}
}

func TestScreen_DirtyRowsOnReplay(t *testing.T) {
	s, _ := replay(t, "w60", 60, 30)
	s.ClearDirty()
	_, _ = s.Write([]byte("\x1b[5;1Hzz"))
	if got := s.DirtyRows(); !slices.Equal(got, []int{4}) {
		t.Errorf("DirtyRows() = %v, want [4]", got)
	}
}

func TestScreen_AllRowsDirtyAfterResizeAndScreenSwitch(t *testing.T) {
	all := []int{0, 1, 2}
	s := screen.NewScreen(10, 2)
	t.Cleanup(func() { _ = s.Close() })
	s.ClearDirty()
	s.Resize(10, 3)
	if got := s.DirtyRows(); !slices.Equal(got, all) {
		t.Errorf("DirtyRows() after resize = %v, want %v", got, all)
	}
	_, _ = s.Write([]byte("\x1b[?1049h"))
	s.ClearDirty()
	_, _ = s.Write([]byte("\x1b[?1049l"))
	if got := s.DirtyRows(); !slices.Equal(got, all) {
		t.Errorf("DirtyRows() after leaving the alternate screen = %v, want %v", got, all)
	}
}

func TestScreen_ScrollbackCappedAt10000(t *testing.T) {
	s := screen.NewScreen(10, 2)
	t.Cleanup(func() { _ = s.Close() })
	var b strings.Builder
	for i := range 10_100 {
		fmt.Fprintf(&b, "\x1b[3%dm%d\x1b[0m\r\n", i%8, i)
	}
	_, _ = s.Write([]byte(b.String()))
	if got := s.ScrollbackLen(); got != 10_000 {
		t.Fatalf("ScrollbackLen() = %d, want 10000", got)
	}
	last := s.StyledScrollback(s.ScrollbackLen() - 1)
	// The screen still shows 10099 and an empty row.
	if !strings.Contains(last, "10098") || !sgr.MatchString(last) {
		t.Errorf("StyledScrollback(last) = %q, want styled 10098", last)
	}
	if got := s.StyledScrollback(10_000); got != "" {
		t.Errorf("StyledScrollback(out of range) = %q, want empty", got)
	}
	if got := s.Pushed(); got != 10_099 {
		t.Errorf("Pushed() = %d, want 10099, counting the dropped lines", got)
	}
}

func TestScreen_StyledViewAndTextSpanScrollbackAndScreen(t *testing.T) {
	s := screen.NewScreen(20, 3)
	t.Cleanup(func() { _ = s.Close() })
	for i := range 6 {
		_, _ = fmt.Fprintf(s, "L%d   x\r\n", i)
	}
	// Scrollback L0-L3, screen L4, L5 and an empty row.
	rows, scrollMax := s.StyledView(2, 4)
	if scrollMax != 4 {
		t.Fatalf("scrollMax = %d, want 4", scrollMax)
	}
	var plain []string
	for _, r := range rows {
		plain = append(plain, strings.TrimRight(ansi.Strip(r), " "))
	}
	if want := []string{"L2   x", "L3   x", "L4   x", "L5   x"}; !slices.Equal(plain, want) {
		t.Errorf("StyledView(2, 4) = %q, want %q", plain, want)
	}
	if rows, _ := s.StyledView(99, 1); ansi.Strip(rows[0]) != "L0   x" {
		t.Errorf("StyledView(99, 1) = %q, want the offset clamped to the oldest line", rows)
	}

	if got, want := s.Text(3, 1, 4, 4), "3   x\nL4"; got != want {
		t.Errorf("Text(3,1 → 4,4) = %q, want %q", got, want)
	}
	if got, want := s.Text(4, 4, 3, 1), "3   x\nL4"; got != want {
		t.Errorf("Text with start and end swapped = %q, want %q", got, want)
	}
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// written closes the screen so every reply has reached the writer.
func written(t *testing.T, s *screen.Screen, b *syncBuffer) string {
	t.Helper()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestScreen_RepliesGoToWriter(t *testing.T) {
	var out syncBuffer
	s := screen.NewScreenWithReplies(10, 2, &out)
	_, _ = s.Write([]byte("\x1b[c"))
	if got := written(t, s, &out); !strings.HasPrefix(got, "\x1b[?") {
		t.Errorf("reply to device attributes = %q, want a DA1 reply", got)
	}
}

func TestScreen_RepliesWithoutWriterNeverBlock(t *testing.T) {
	s := screen.NewScreen(10, 2)
	t.Cleanup(func() { _ = s.Close() })
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 1000 {
			_, _ = s.Write([]byte("\x1b[c\x1b[6n"))
		}
		s.SendKey(screen.Key{Code: 'a', Text: "a"})
		s.Paste("p")
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("writes with replies blocked")
	}
}

func TestScreen_PasteBracketedOnlyWhenEnabled(t *testing.T) {
	cases := []struct {
		name, modes, want string
	}{
		{"off", "", "hi"},
		{"on", "\x1b[?2004h", "\x1b[200~hi\x1b[201~"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out syncBuffer
			s := screen.NewScreenWithReplies(10, 2, &out)
			_, _ = s.Write([]byte(c.modes))
			s.Paste("hi")
			if got := written(t, s, &out); got != c.want {
				t.Errorf("Paste wrote %q, want %q", got, c.want)
			}
		})
	}
}

func TestScreen_SendKeyFollowsCursorKeyMode(t *testing.T) {
	cases := []struct {
		name, modes, want string
	}{
		{"normal", "", "\x1b[A"},
		{"application", "\x1b[?1h", "\x1bOA"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out syncBuffer
			s := screen.NewScreenWithReplies(10, 2, &out)
			_, _ = s.Write([]byte(c.modes))
			s.SendKey(screen.Key{Code: screen.KeyUp})
			if got := written(t, s, &out); got != c.want {
				t.Errorf("SendKey(up) wrote %q, want %q", got, c.want)
			}
		})
	}
}

func TestScreen_SendKeyTypesShiftedText(t *testing.T) {
	cases := []struct {
		name string
		key  screen.Key
		want string
	}{
		{"shift letter", screen.Key{Code: 'h', Text: "H", Mod: screen.ModShift}, "H"},
		{"caps lock", screen.Key{Code: 'h', Text: "H", Mod: screen.ModCapsLock}, "H"},
		{"alt shift letter", screen.Key{Code: 'h', Text: "H", Mod: screen.ModShift | screen.ModAlt}, "\x1bH"},
		{"ctrl letter", screen.Key{Code: 'c', Mod: screen.ModCtrl}, "\x03"},
		{"plain letter", screen.Key{Code: 'h', Text: "h"}, "h"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out syncBuffer
			s := screen.NewScreenWithReplies(10, 2, &out)
			s.SendKey(c.key)
			if got := written(t, s, &out); got != c.want {
				t.Errorf("SendKey(%+v) wrote %q, want %q", c.key, got, c.want)
			}
		})
	}
}

func TestTitle_Replay(t *testing.T) {
	for _, r := range replays {
		t.Run(r.name, func(t *testing.T) {
			_, raw := replay(t, r.name, r.cols, r.rows)
			if got, ok := screen.Title(raw); !ok || got != r.title {
				t.Errorf("Title() = %q, %v; want %q", got, ok, r.title)
			}
		})
	}
}

func TestTitle(t *testing.T) {
	cases := []struct {
		name, raw, want string
		ok              bool
	}{
		{"BEL terminator", "\x1b]0;✳ Claude Code\x07", "✳ Claude Code", true},
		{"ST terminator", "\x1b]0;◐ Working\x1b\\", "◐ Working", true},
		{"last wins", "\x1b]0;one\x07text\x1b]0;two\x07", "two", true},
		{"unterminated last falls back", "\x1b]0;one\x07\x1b]0;tw", "one", true},
		{"empty title", "\x1b]0;\x07", "", true},
		{"none", "plain \x1b]2;other\x07", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got, ok := screen.Title([]byte(c.raw)); got != c.want || ok != c.ok {
				t.Errorf("Title(%q) = %q, %v; want %q, %v", c.raw, got, ok, c.want, c.ok)
			}
		})
	}
}
