package harness_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/harness"
)

const (
	rst  = "\x1b[m"
	dim  = "\x1b[2m"
	bold = "\x1b[1m"
	rev  = "\x1b[7m"
	rule = dim + "────────────────────────" + rst
)

// plainOf drops the styling, as a text read of the same screen would.
func plainOf(rows []string) []string {
	re := regexp.MustCompile("\x1b\\[[0-9;]*m")
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = re.ReplaceAllString(r, "")
	}
	return out
}

func TestTyping_InputIsEmpty(t *testing.T) {
	tests := []struct {
		name  string
		kind  string
		rows  []string
		want  harness.InputState
		plain bool // also assert the unstyled read of rows is undecided
	}{
		{
			name: "claude placeholder",
			kind: "claude",
			rows: []string{rule, "❯ " + rev + "T" + rst + dim + "ry \"fix the bug\"" + rst, rule, dim + "  ? for shortcuts" + rst},
			want: harness.InputEmpty, plain: true,
		},
		{
			name: "claude bare prompt",
			kind: "claude",
			rows: []string{rule, "❯ " + rev + " " + rst, rule},
			want: harness.InputEmpty, plain: true,
		},
		{
			// Claude draws a no-break space after the marker and the next-prompt
			// suggestion dim, with the terminal cursor rather than a reverse cell.
			name: "claude suggestion after a no-break space",
			kind: "claude",
			rows: []string{rule, "❯\u00a0" + dim + "/clear" + rst, rule},
			want: harness.InputEmpty, plain: true,
		},
		{
			name: "claude draft",
			kind: "claude",
			rows: []string{rule, "❯ fix the bug" + rev + " " + rst, rule, dim + "  ? for shortcuts" + rst},
			want: harness.InputNotEmpty, plain: true,
		},
		{
			name: "claude multi-line draft",
			kind: "claude",
			rows: []string{rule, "❯ first line", "  second line" + rev + " " + rst, rule},
			want: harness.InputNotEmpty, plain: true,
		},
		{
			name: "claude draft starting with a blank line",
			kind: "claude",
			rows: []string{rule, "❯ ", "  second line", rule},
			want: harness.InputNotEmpty, plain: true,
		},
		{
			name: "claude draft that reads like the placeholder",
			kind: "claude",
			rows: []string{rule, "❯ Try \"fix the bug\"", rule},
			want: harness.InputNotEmpty, plain: true,
		},
		{
			name: "claude scrollback prompt above the live one",
			kind: "claude",
			rows: []string{"❯ earlier prompt", "⏺ done", rule, "❯ " + dim + "Try something" + rst, rule},
			want: harness.InputEmpty, plain: true,
		},
		{
			name: "claude no input box",
			kind: "claude",
			rows: []string{dim + "loading" + rst},
			want: harness.InputUndecided,
		},
		{
			name: "codex placeholder",
			kind: "codex",
			rows: []string{bold + "›" + rst + " " + rev + "I" + rst + dim + "mprove documentation in @filename" + rst, "", dim + "  gpt-5 · ~/dev" + rst},
			want: harness.InputEmpty, plain: true,
		},
		{
			name: "codex draft",
			kind: "codex",
			rows: []string{bold + "›" + rst + " fix the bug", "", dim + "  gpt-5 · ~/dev" + rst},
			want: harness.InputNotEmpty, plain: true,
		},
		{
			name: "codex multi-line draft",
			kind: "codex",
			rows: []string{bold + "›" + rst + " first line", "  second line", "", dim + "  gpt-5 · ~/dev" + rst},
			want: harness.InputNotEmpty, plain: true,
		},
		{
			name: "codex unbold marker is not the input",
			kind: "codex",
			rows: []string{"› quoted in the transcript", dim + "  footer" + rst},
			want: harness.InputUndecided,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ty, err := harness.LookupTyping(agentKind(tt.kind))
			if err != nil {
				t.Fatal(err)
			}
			if got := ty.InputIsEmpty(tt.rows); got != tt.want {
				t.Errorf("styled: got %v, want %v", got, tt.want)
			}
			if tt.plain {
				if got := ty.InputIsEmpty(plainOf(tt.rows)); got != harness.InputUndecided {
					t.Errorf("unstyled: got %v, want InputUndecided", got)
				}
			}
		})
	}
}

func TestTyping_ClearKey(t *testing.T) {
	for _, kind := range []string{"claude", "codex"} {
		ty, err := harness.LookupTyping(agentKind(kind))
		if err != nil {
			t.Fatal(err)
		}
		if got := ty.ClearKey(); got != "\x15" {
			t.Errorf("%s ClearKey = %q, want Ctrl+U", kind, got)
		}
	}
}

// vimConfig points claude's config at a temp dir whose .claude.json has mode.
func vimConfig(t *testing.T, mode string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	if mode == "" {
		return
	}
	if err := os.WriteFile(filepath.Join(dir, ".claude.json"), []byte(`{"editorMode":"`+mode+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
}

// screenIO serves rows[i] on the i-th read (the last one repeats) and records
// the keys pressed. An `i` press advances to the next row set.
type screenIO struct {
	rows  [][]string
	reads int
	keys  []string
}

func (s *screenIO) io() harness.TypingIO {
	return harness.TypingIO{
		Rows: func() ([]string, error) {
			s.reads++
			return s.rows[min(len(s.keys), len(s.rows)-1)], nil
		},
		SendKeys: func(k string) error { s.keys = append(s.keys, k); return nil },
	}
}

var (
	normalRows = []string{rule, "❯ " + rev + " " + rst, rule, dim + "  ? for shortcuts" + rst}
	insertRows = []string{rule, "❯ " + rev + " " + rst, rule, dim + "  -- INSERT --" + rst}
)

func TestClaudeReadyToType_Vim(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		rows     [][]string
		want     bool
		wantKeys []string
	}{
		{name: "vim in NORMAL presses i then sees INSERT", mode: "vim", rows: [][]string{normalRows, insertRows}, want: true, wantKeys: []string{"i"}},
		{name: "vim already in INSERT presses nothing", mode: "vim", rows: [][]string{insertRows}, want: true},
		{name: "vim never reaching INSERT gives up", mode: "vim", rows: [][]string{normalRows}, want: false, wantKeys: []string{"i"}},
		{name: "marker above the input box is not the footer", mode: "vim", rows: [][]string{{"-- INSERT --", rule, "❯ ", rule}}, want: false, wantKeys: []string{"i"}},
		{name: "normal mode presses nothing", mode: "normal", rows: [][]string{normalRows}, want: true},
		{name: "no editorMode presses nothing", mode: "", rows: [][]string{normalRows}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vimConfig(t, tt.mode)
			ty, _ := harness.LookupTyping(harness.Claude)
			s := &screenIO{rows: tt.rows}
			if got := ty.ReadyToType(s.io()); got != tt.want {
				t.Errorf("ReadyToType = %v, want %v", got, tt.want)
			}
			if !slices.Equal(s.keys, tt.wantKeys) {
				t.Errorf("keys = %q, want %q", s.keys, tt.wantKeys)
			}
		})
	}
}

func TestClaudeReadyToType_ReadsEditorModeEveryCall(t *testing.T) {
	vimConfig(t, "normal")
	ty, _ := harness.LookupTyping(harness.Claude)
	s := &screenIO{rows: [][]string{normalRows}}
	if !ty.ReadyToType(s.io()) || len(s.keys) != 0 {
		t.Fatalf("normal mode: keys = %q", s.keys)
	}
	if err := os.WriteFile(filepath.Join(os.Getenv("CLAUDE_CONFIG_DIR"), ".claude.json"), []byte(`{"editorMode":"vim"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s = &screenIO{rows: [][]string{normalRows, insertRows}}
	if !ty.ReadyToType(s.io()) || !slices.Equal(s.keys, []string{"i"}) {
		t.Fatalf("after toggle to vim: keys = %q", s.keys)
	}
}

func TestClaudeReadyToType_SettingsJSONAlsoCounts(t *testing.T) {
	vimConfig(t, "")
	if err := os.WriteFile(filepath.Join(os.Getenv("CLAUDE_CONFIG_DIR"), "settings.json"), []byte(`{"editorMode":"vim"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	ty, _ := harness.LookupTyping(harness.Claude)
	s := &screenIO{rows: [][]string{normalRows, insertRows}}
	if !ty.ReadyToType(s.io()) || !slices.Equal(s.keys, []string{"i"}) {
		t.Fatalf("keys = %q", s.keys)
	}
}

func TestCodexReadyToType_IsANoOp(t *testing.T) {
	vimConfig(t, "vim")
	ty, _ := harness.LookupTyping(harness.Codex)
	s := &screenIO{rows: [][]string{normalRows}}
	if !ty.ReadyToType(s.io()) || len(s.keys) != 0 || s.reads != 0 {
		t.Fatalf("codex touched the screen: keys = %q, reads = %d", s.keys, s.reads)
	}
}

func TestTyping_SubmitKeyAndPasteRule(t *testing.T) {
	tests := []struct {
		name      string
		kind      agentrunner.Kind
		working   bool
		text      string
		wantKey   string
		wantPaste bool
	}{
		{"claude idle", harness.Claude, false, "one\ntwo", "\r", false},
		{"claude working", harness.Claude, true, "one", "\r", false},
		{"codex idle single line", harness.Codex, false, "one", "\r", false},
		{"codex working queues with tab", harness.Codex, true, "one", "\t", false},
		{"codex multi-line pastes", harness.Codex, false, "one\ntwo", "\r", true},
		{"codex working multi-line", harness.Codex, true, "one\ntwo", "\t", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ty, err := harness.LookupTyping(tt.kind)
			if err != nil {
				t.Fatal(err)
			}
			if got := ty.SubmitKey(tt.working); got != tt.wantKey {
				t.Errorf("SubmitKey(%v) = %q, want %q", tt.working, got, tt.wantKey)
			}
			if got := ty.PastesText(tt.text); got != tt.wantPaste {
				t.Errorf("PastesText(%q) = %v, want %v", tt.text, got, tt.wantPaste)
			}
		})
	}
}

func TestLookupTyping_UnknownKindIsAnError(t *testing.T) {
	if _, err := harness.LookupTyping("nope"); err == nil {
		t.Fatal("LookupTyping(nope) succeeded")
	}
}

func agentKind(s string) agentrunner.Kind { return agentrunner.Kind(s) }
