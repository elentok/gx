package screen_test

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

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
