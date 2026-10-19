package harness_test

import (
	"regexp"
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

func TestTyping_ClearKeyAndReady(t *testing.T) {
	for _, kind := range []string{"claude", "codex"} {
		ty, err := harness.LookupTyping(agentKind(kind))
		if err != nil {
			t.Fatal(err)
		}
		if got := ty.ClearKey(); got != "\x15" {
			t.Errorf("%s ClearKey = %q, want Ctrl+U", kind, got)
		}
		if !ty.ReadyToType() {
			t.Errorf("%s ReadyToType = false", kind)
		}
	}
}

func TestLookupTyping_UnknownKindIsAnError(t *testing.T) {
	if _, err := harness.LookupTyping("nope"); err == nil {
		t.Fatal("LookupTyping(nope) succeeded")
	}
}

func agentKind(s string) agentrunner.Kind { return agentrunner.Kind(s) }
