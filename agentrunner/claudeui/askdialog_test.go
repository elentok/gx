package claudeui

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

// require mirrors the few testify calls this file uses; the module has no testify.
var require askRequire

type askRequire struct{}

func (askRequire) NoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func (askRequire) Equal(t *testing.T, want, got any) {
	t.Helper()
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("want %#v, got %#v", want, got)
	}
}

func (askRequire) NotEqual(t *testing.T, a, b any) {
	t.Helper()
	if reflect.DeepEqual(a, b) {
		t.Fatalf("both %#v", a)
	}
}

func (askRequire) NotEmpty(t *testing.T, s string) {
	t.Helper()
	if s == "" {
		t.Fatal("empty")
	}
}

func (askRequire) True(t *testing.T, b bool) {
	t.Helper()
	if !b {
		t.Fatal("want true")
	}
}

func (askRequire) False(t *testing.T, b bool) {
	t.Helper()
	if b {
		t.Fatal("want false")
	}
}

const askFooter = "Enter to select · Tab/Arrow keys to navigate · Esc to cancel"

func askScreen(lines ...string) []string {
	return strings.Split(strings.Join(lines, "\n"), "\n")
}

func TestParseAskDialog(t *testing.T) {
	fileRows, err := os.ReadFile("testdata/dialog-askuserquestion.screen.txt")
	require.NoError(t, err)

	threeQ := func(cursorLine string) []string {
		return askScreen(
			"←  ☒ Color  ☐ Toppings  ☐ Name  ✔ Submit  →",
			"",
			"Which toppings?",
			"❯ 1. [ ] Cheese",
			"         Cheese",
			"  2. [✔] Ham",
			"         Ham",
			"  3. [ ] Type something",
			cursorLine,
			"────────────",
			"  4. Chat about this",
			"",
			askFooter,
		)
	}

	tests := []struct {
		name   string
		rows   []string
		ok     bool
		want   AskDialog
		assert func(t *testing.T, d AskDialog)
	}{
		{
			name: "file fixture single select",
			rows: strings.Split(string(fileRows), "\n"),
			ok:   true,
			want: AskDialog{
				Tabs: []AskTab{{Label: "Color"}}, Question: "Which color do you want?",
				Options: []AskOption{{Label: "Red"}, {Label: "Blue"}, {Label: "Type something.", Other: true}},
				Cursor:  AskCursorOption,
			},
		},
		{
			name: "single select with descriptions and chat row",
			rows: askScreen(
				"←  ☐ Color  ☐ Name  ✔ Submit  →",
				"",
				"Which color?",
				"  1. Red",
				"     Red",
				"❯ 2. Blue",
				"     Blue",
				"  3. Type something.",
				"────────────",
				"  4. Chat about this",
				"",
				askFooter,
			),
			ok: true,
			want: AskDialog{
				Tabs: []AskTab{{Label: "Color"}, {Label: "Name"}}, SubmitTab: true, Question: "Which color?",
				Options:    []AskOption{{Label: "Red", Description: "Red"}, {Label: "Blue", Description: "Blue"}, {Label: "Type something.", Other: true}},
				ChatNumber: 4, Cursor: AskCursorOption, CursorIndex: 1,
			},
		},
		{
			name: "multi select, cursor on option",
			rows: threeQ("     Next"),
			ok:   true,
			want: AskDialog{
				Tabs: []AskTab{{Label: "Color", Answered: true}, {Label: "Toppings"}, {Label: "Name"}}, SubmitTab: true,
				Question: "Which toppings?", Multi: true,
				Options: []AskOption{
					{Label: "Cheese", Description: "Cheese"},
					{Label: "Ham", Description: "Ham", Checked: true},
					{Label: "Type something", Other: true},
				},
				Advance: "Next", ChatNumber: 4, Cursor: AskCursorOption,
			},
		},
		{
			name: "multi select, cursor on advance row",
			rows: strings.Split(strings.Replace(strings.Join(threeQ("❯ Submit"), "\n"), "❯ 1.", "  1.", 1), "\n"),
			ok:   true,
			assert: func(t *testing.T, d AskDialog) {
				require.Equal(t, AskCursorAdvance, d.Cursor)
				require.Equal(t, "Submit", d.Advance)
			},
		},
		{
			name: "cursor on chat row",
			rows: strings.Split(strings.Replace(strings.Replace(strings.Join(threeQ("     Next"), "\n"), "❯ 1.", "  1.", 1), "  4. Chat", "❯ 4. Chat", 1), "\n"),
			ok:   true,
			assert: func(t *testing.T, d AskDialog) {
				require.Equal(t, AskCursorChat, d.Cursor)
			},
		},
		{
			name: "other row typed",
			rows: askScreen(
				"←  ☐ Color  ☐ Name  ✔ Submit  →",
				"",
				"Which color?",
				"  1. Red",
				"  2. Blue",
				"❯ 3. Purple",
				"",
				askFooter+" · ctrl+g to edit in Vim",
			),
			ok: true,
			want: AskDialog{
				Tabs: []AskTab{{Label: "Color"}, {Label: "Name"}}, SubmitTab: true, Question: "Which color?",
				Options: []AskOption{{Label: "Red"}, {Label: "Blue"}, {Label: "Purple", Other: true}},
				Cursor:  AskCursorOption, CursorIndex: 2,
			},
		},
		{
			name: "review screen",
			rows: askScreen(
				"←  ☒ Color  ☒ Toppings  ✔ Submit  →",
				"",
				"Review your answers",
				"",
				" ● Which color?",
				"   → Purple",
				" ● Which toppings?",
				"   → Egg, Sausage",
				"",
				"Ready to submit your answers?",
				"",
				"❯ 1. Submit answers",
				"  2. Cancel",
				"",
				askFooter,
			),
			ok: true,
			want: AskDialog{
				Tabs:      []AskTab{{Label: "Color", Answered: true}, {Label: "Toppings", Answered: true}},
				SubmitTab: true, Review: true, Question: "Ready to submit your answers?",
				ReviewItems: []AskReviewItem{{"Which color?", "Purple"}, {"Which toppings?", "Egg, Sausage"}},
				Options:     []AskOption{{Label: "Submit answers"}, {Label: "Cancel"}},
				Cursor:      AskCursorOption,
			},
		},
		{
			name: "wrapped 80 columns with gutter",
			rows: askScreen(
				"← ☐ Strategy  ☐ Window  ✔ Submit →",
				"",
				"│ Which deployment strategy should we",
				"│ use for the release?",
				"",
				"❯ 1. Roll out gradually to a small",
				"     share of users first",
				"     Start with canary traffic and",
				"     widen over a day",
				"  2. Everything at once",
				"     Fastest",
				"  3. Type something.",
				"─────────────",
				"  4. Chat about this",
				"",
				"Enter to select · Tab/Arrow keys to navigate · Esc to",
				"cancel",
			),
			ok: true,
			want: AskDialog{
				Tabs: []AskTab{{Label: "Strategy"}, {Label: "Window"}}, SubmitTab: true,
				Question: "Which deployment strategy should we use for the release?",
				Options: []AskOption{
					{Label: "Roll out gradually to a small", Description: "share of users first Start with canary traffic and widen over a day"},
					{Label: "Everything at once", Description: "Fastest"},
					{Label: "Type something.", Other: true},
				},
				ChatNumber: 4, Cursor: AskCursorOption,
			},
		},
		{
			name: "scrolled 80x16 is unreadable",
			rows: askScreen(
				"↑ 2. Short",
				"❯ 3. Medium",
				"↓ 4. Canary",
				"Enter to select · Tab/Arrow keys to navigate · Esc to",
				"cancel",
			),
		},
		{
			name: "scroll markers with tab bar are unreadable",
			rows: askScreen(
				"←  ☐ Color  ☐ Name  ✔ Submit  →",
				"Which color?",
				"↑ 2. Blue",
				"❯ 3. Green",
				"↓ 4. Type something.",
				askFooter,
			),
		},
		{
			name: "no cursor row is unreadable",
			rows: askScreen("☐ Color", "", "Which color?", "  1. Red", "  2. Type something.", "", askFooter),
		},
		{
			name: "missing tab bar is unreadable",
			rows: askScreen("Which color?", "❯ 1. Red", "  2. Type something.", "", askFooter),
		},
		{
			name: "unknown footer is unreadable",
			rows: askScreen("☐ Color", "", "Which color?", "❯ 1. Red", "  2. Type something.", "", "Press any key to continue"),
		},
		{
			name: "unparsable row after the rule is unreadable",
			rows: askScreen("☐ Color", "", "Which color?", "❯ 1. Red", "  2. Type something.", "────────", "  something else", "", askFooter),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, ok := ParseAskDialog(tt.rows)
			require.Equal(t, tt.ok, ok)
			if !ok {
				require.Equal(t, AskDialog{}, d)
				return
			}
			require.NotEmpty(t, d.ID)
			if tt.assert != nil {
				tt.assert(t, d)
				return
			}
			d.ID = ""
			require.Equal(t, tt.want, d)
		})
	}
}

func TestParseAskDialogIDIgnoresAnswersAndCursor(t *testing.T) {
	screen := func(cursor1, cursor2, box, other string) []string {
		return askScreen(
			"←  ☐ Toppings  ✔ Submit  →",
			"",
			"Which toppings?",
			cursor1+"1. "+box+" Cheese",
			cursor2+"2. "+box+" "+other,
			"     Next",
			askFooter,
		)
	}
	a, ok := ParseAskDialog(screen("❯ ", "  ", "[ ]", "Type something"))
	require.True(t, ok)
	b, ok := ParseAskDialog(screen("  ", "❯ ", "[✔]", "Sausage"))
	require.True(t, ok)
	require.Equal(t, a.ID, b.ID)

	c, ok := ParseAskDialog(strings.Split(strings.Replace(strings.Join(screen("❯ ", "  ", "[ ]", "Type something"), "\n"), "Which", "What", 1), "\n"))
	require.True(t, ok)
	require.NotEqual(t, a.ID, c.ID)
}

func TestParseDialogAskUserQuestionNotApprovable(t *testing.T) {
	raw, err := os.ReadFile("testdata/dialog-askuserquestion.screen.txt")
	require.NoError(t, err)
	require.False(t, ParseDialog(strings.Split(string(raw), "\n")).Approvable)
}
