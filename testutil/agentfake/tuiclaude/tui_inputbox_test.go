package tuiclaude

import . "github.com/elentok/gx/testutil/agentfake"

import (
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/agentrunner/screen"
	"github.com/elentok/gx/harness"
)

const truecolorEnv = "COLORTERM=truecolor"

var helpColor = "38;2;177;185;249"

// replay is what the fake's screen shows once its bytes are replayed.
func (f *tuiFake) replay() *screen.Screen {
	f.t.Helper()
	s := screen.NewScreen(120, 40)
	if _, err := s.Write([]byte(f.screen.String())); err != nil {
		f.t.Fatal(err)
	}
	return s
}

func (f *tuiFake) rows() []string { return f.replay().Rows() }

// waitRows waits until some row contains text, and returns the rows.
func (f *tuiFake) waitRows(text string) []string {
	f.t.Helper()
	var rows []string
	f.waitFor("a row with "+text, func() bool {
		rows = f.rows()
		return slices.ContainsFunc(rows, func(r string) bool { return strings.Contains(r, text) })
	})
	return rows
}

func (f *tuiFake) styledRow(prefix string) string {
	f.t.Helper()
	for _, r := range f.replay().StyledRows() {
		if strings.Contains(r, prefix) {
			return r
		}
	}
	return ""
}

func (f *tuiFake) hasRow(text string) bool {
	return slices.ContainsFunc(f.rows(), func(r string) bool { return strings.Contains(r, text) })
}

func (f *tuiFake) idleStatus() {
	f.t.Helper()
	if m := f.pidFile(); m["status"] != StatusIdle {
		f.t.Fatalf("status = %v, want idle; screen %q", m["status"], f.screen.String())
	}
}

func TestInputBox_EveryIdleDraftIsABox(t *testing.T) {
	long := "a long draft that wraps over a few rows"
	for _, tt := range []struct {
		name, keys string
		want       harness.InputState
	}{
		{"empty placeholder", "", harness.InputEmpty},
		{"typed", "hello", harness.InputNotEmpty},
		{"multi-line", "one\ntwo", harness.InputNotEmpty},
		{"wrapped", long, harness.InputNotEmpty},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := startTUIFake(t, tuiLaunchArgs, truecolorEnv)
			f.waitStatus(StatusIdle, "")
			want := "Try"
			if tt.keys != "" {
				f.typeRaw(tt.keys)
				want = strings.Fields(tt.keys)[len(strings.Fields(tt.keys))-1]
			}
			rows := f.waitRows(want)
			// claudeui.InputBox wants the ❯ row between two rules, so it reads a
			// single-row draft only; a longer one is framed by rules all the same.
			first := slices.IndexFunc(rows, func(r string) bool { return strings.HasPrefix(r, "❯") })
			last := first
			for strings.HasPrefix(rows[last+1], "  ") {
				last++
			}
			if first < 2 || !strings.HasPrefix(rows[first-1], "───") || !strings.HasPrefix(rows[last+1], "───") {
				t.Errorf("draft rows %d-%d are not framed by rules: %q", first, last, rows)
			}
			if single := first == last; single && !claudeui.InputBox(rows) {
				t.Errorf("no input box in %q", rows)
			}
			for _, r := range rows {
				if strings.HasPrefix(r, "> ") {
					t.Errorf("echoed as %q", r)
				}
			}
			if got := harness.ClaudeTyping().InputIsEmpty(f.replay().StyledRows()); got != tt.want {
				t.Errorf("InputIsEmpty = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInputBox_WrappedDraftStaysInsideTheBox(t *testing.T) {
	f := startTUIFake(t, tuiLaunchArgs)
	f.waitStatus(StatusIdle, "")
	f.typeRaw("alpha beta gamma delta epsilon zeta")
	rows := f.waitRows("zeta")
	first := slices.IndexFunc(rows, func(r string) bool { return strings.HasPrefix(r, "❯ alpha") })
	if first < 0 || !strings.HasPrefix(rows[first+1], "  ") || !strings.HasPrefix(rows[first+2], "───") {
		t.Errorf("rows = %q", rows)
	}
}

func TestInputBox_KnownSlashCommandIsHighlightedByTheColourSwitch(t *testing.T) {
	for _, tt := range []struct {
		name, text string
		env        []string
		want       string
	}{
		{"truecolor", "/help me", []string{truecolorEnv}, helpColor},
		{"256", "/help", []string{"COLORTERM=yes"}, "38;5;147"},
		{"colourless", "/help", []string{"COLORTERM=", "TERM=dumb"}, ""},
		{"unknown", "/zzz", []string{truecolorEnv, TUIUnknownSkillEnv + "=1"}, ""},
		{"plain", "help", []string{truecolorEnv}, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := startTUIFake(t, tuiLaunchArgs, tt.env...)
			f.waitStatus(StatusIdle, "")
			f.typeRaw(tt.text)
			f.waitRows(strings.Fields(tt.text)[0])
			row := f.styledRow("❯")
			colored := strings.Contains(row, "38;2;") || strings.Contains(row, "38;5;")
			switch {
			case tt.want == "" && colored:
				t.Errorf("row %q is coloured", row)
			case tt.want != "" && !strings.Contains(row, tt.want):
				t.Errorf("row %q lacks %s", row, tt.want)
			}
			if strings.Contains(tt.text, " me") && strings.Contains(strings.SplitN(row, "me", 2)[0], "m me") {
				t.Errorf("arguments are coloured: %q", row)
			}
		})
	}
}

func TestInputBox_SlashDraftShowsAMenuThatEscClosesKeepingTheText(t *testing.T) {
	f := startTUIFake(t, tuiLaunchArgs, truecolorEnv)
	f.waitStatus(StatusIdle, "")
	f.typeRaw("/co")
	rows := f.waitRows("❯ /compact")
	for _, want := range []string{"    /cost", "    /copy"} {
		if !slices.ContainsFunc(rows, func(r string) bool { return strings.HasPrefix(r, want) }) {
			t.Errorf("no menu row %q in %q", want, rows)
		}
	}
	if f.hasRow("/help  ") {
		t.Errorf("menu lists a command that does not match: %q", rows)
	}

	f.typeRaw("\x1b")
	f.waitFor("the menu to close", func() bool { return !f.hasRow("/compact ") })
	if !f.hasRow("❯ /co") {
		t.Errorf("Esc dropped the text: %q", f.rows())
	}
	f.typeRaw("p")
	f.waitRows("/cop")
	if !f.hasRow("/copy ") {
		t.Errorf("menu did not return after a key: %q", f.rows())
	}
}

func TestInputBox_HiddenCharacterSwallowsTheFirstEnterOfASlashOrBangDraft(t *testing.T) {
	for _, tt := range []struct {
		name, raw, cleaned, notice string
	}{
		{"zero width space", "/he​lp", "/help", "Removed 1 invisible character · review and press Enter to send"},
		{"U+1BCA0 within 20", "/he\U0001BCA0lp", "/help", "Removed 1 invisible character"},
		{"two characters", "/h​​elp", "/help", "Removed 2 invisible characters"},
		{"bang", "!ls​ -a", "!ls -a", "Removed 1 invisible character"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := startTUIFake(t, tuiLaunchArgs, truecolorEnv)
			f.waitStatus(StatusIdle, "")
			f.control(Control{Action: ActionBusy})
			f.paste(tt.raw)
			f.waitRows(strings.Fields(tt.raw)[0][:2])
			if row := f.styledRow("❯"); strings.Contains(row, helpColor) {
				t.Errorf("raw paste is coloured: %q", row)
			}

			f.typeRaw("\r")
			f.waitRows(tt.notice)
			f.idleStatus()
			if !f.hasRow("❯ " + tt.cleaned) {
				t.Errorf("box is not the cleaned text: %q", f.rows())
			}
			if tt.cleaned == "/help" && !strings.Contains(f.styledRow("❯"), helpColor) {
				t.Errorf("cleaned command is not coloured: %q", f.styledRow("❯"))
			}

			f.typeRaw("\r")
			f.waitStatus(StatusBusy, "")
		})
	}
}

func TestInputBox_SwallowedEnterWritesNoRecord(t *testing.T) {
	f := startTUIFake(t, tuiLaunchArgs)
	f.writeWorkerConfig(WorkerConfig{Chat: true})
	f.waitStatus(StatusIdle, "")
	f.paste("/he​lp")
	f.typeRaw("\r")
	f.waitRows("Removed 1 invisible character")
	if lines := f.transcriptLines(); len(lines) != 0 {
		t.Errorf("transcript after the swallowed Enter = %q", lines)
	}
}

func TestInputBox_PlainTextWithAHiddenCharacterIsCleanedAndSentAtOnce(t *testing.T) {
	f := startTUIFake(t, tuiLaunchArgs)
	f.writeWorkerConfig(WorkerConfig{Chat: true})
	f.waitStatus(StatusIdle, "")
	f.paste("he​llo")
	f.typeRaw("\r")
	f.waitStatus(StatusBusy, "")
	f.waitStatus(StatusIdle, "")
	if got := strings.Join(f.transcriptLines(), "\n"); !strings.Contains(got, `"hello"`) {
		t.Errorf("transcript = %s", got)
	}
}

func TestInputBox_JoinersAndLineSeparatorsAreNotHidden(t *testing.T) {
	for _, tt := range []struct{ name, raw string }{
		{"ZWJ in an emoji sequence", "/hi 👨‍👩"},
		{"variation selector", "/hi ❤️!"},
		{"U+2028", "/hi a b"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := startTUIFake(t, tuiLaunchArgs)
			f.waitStatus(StatusIdle, "")
			f.control(Control{Action: ActionBusy})
			f.paste(tt.raw)
			f.waitRows("/hi")
			f.typeRaw("\r")
			f.waitStatus(StatusBusy, "")
		})
	}
}

func TestInputBox_SwallowEnterControlKeepsTheTextOnce(t *testing.T) {
	f := startTUIFake(t, tuiLaunchArgs)
	f.waitStatus(StatusIdle, "")
	f.control(Control{Action: ActionSwallowEnter})
	f.typeLine("hello")
	f.waitRows("❯ hello")
	f.idleStatus()
	if f.hasRow("Removed") {
		t.Errorf("a notice was drawn: %q", f.rows())
	}
	f.typeRaw("\r")
	f.waitStatus(StatusBusy, "")
}

func TestInputBox_LongPasteIsAChipAndSubmitsWhole(t *testing.T) {
	f := startTUIFake(t, tuiLaunchArgs)
	f.writeWorkerConfig(WorkerConfig{Chat: true})
	f.waitStatus(StatusIdle, "")
	body := strings.Repeat("0123456789\n", 60)
	f.paste(body)
	// The 20-column fallback wraps the chip over rows, so read them joined.
	f.waitFor("the paste chip", func() bool {
		return strings.Contains(claudeui.Normalize(f.rows()...), PastedChip(1, body))
	})
	if want := "[Pasted text #1 +60 lines]"; PastedChip(1, body) != want {
		t.Errorf("chip = %q, want %q", PastedChip(1, body), want)
	}
	if f.hasRow("0123456789") {
		t.Errorf("the pasted text is on screen: %q", f.rows())
	}
	f.typeRaw("\r")
	f.waitStatus(StatusBusy, "")
	f.waitStatus(StatusIdle, "")
	if got := strings.Join(f.transcriptLines(), "\n"); !strings.Contains(got, strings.ReplaceAll(body[:22], "\n", `\n`)) || strings.Contains(got, "Pasted text") {
		t.Errorf("transcript = %s", got)
	}
}

func TestInputBox_ShortPasteStaysPlain(t *testing.T) {
	f := startTUIFake(t, tuiLaunchArgs)
	f.waitStatus(StatusIdle, "")
	f.paste(strings.Repeat("x", 512))
	f.waitRows("xxxx")
	if f.hasRow("Pasted text") {
		t.Errorf("a 512-byte paste became a chip: %q", f.rows())
	}
}
