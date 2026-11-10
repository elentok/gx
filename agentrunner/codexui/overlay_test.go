package codexui_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/elentok/gx/agentrunner/codexui"
)

func TestParseOverlay(t *testing.T) {
	t.Parallel()
	const three = `  Question 2/3 (2 unanswered)
  Shape
  › 1. Round              Round shape.
    2. Oval               Oval shape.
    3. Square             Square shape.
    4. None of the above  Optionally, add details in notes (tab)
  tab to add notes | enter to submit answer | ←/→ to navigate questions | esc to interrupt`
	none := "None of the above"
	for _, tc := range []struct {
		name   string
		screen string
		want   codexui.Overlay
		ok     bool
	}{
		{"three options", three, codexui.Overlay{
			Number: 2, Total: 3, Unanswered: 2, Question: "Shape",
			Labels: []string{"Round", "Oval", "Square", none}, Rows: 4, Cursor: 1,
		}, true},
		{"two options, blank line before footer", `  Question 1/1 (1 unanswered)
  Which color?

  › 1. Red                Choose red.
    2. Blue               Choose blue.
    3. None of the above  Optionally, add details in notes (tab)

  tab to add notes | enter to submit answer | esc to interrupt`, codexui.Overlay{
			Number: 1, Total: 1, Unanswered: 1, Question: "Which color?",
			Labels: []string{"Red", "Blue", none}, Rows: 3, Cursor: 1,
		}, true},
		{"wrapped question", `  Question 1/2 (2 unanswered)
  Which of these
  colors   do you want?
  › 1. Red   Choose red.
    2. None of the above  Optionally, add details in notes (tab)
  tab to add notes | enter to submit answer | esc to interrupt`, codexui.Overlay{
			Number: 1, Total: 2, Unanswered: 2, Question: "Which of these colors do you want?",
			Labels: []string{"Red", none}, Rows: 2, Cursor: 1,
		}, true},
		{"last question footer", strings.Replace(strings.Replace(three, "2/3 (2", "3/3 (1", 1), "submit answer", "submit all", 1),
			codexui.Overlay{
				Number: 3, Total: 3, Unanswered: 1, Question: "Shape",
				Labels: []string{"Round", "Oval", "Square", none}, Rows: 4, Cursor: 1, LastQuestion: true,
			}, true},
		{"late redraw keeps the old header", "  Question 1/3 (3 unanswered)\n  old\n" + three, codexui.Overlay{
			Number: 2, Total: 3, Unanswered: 2, Question: "Shape",
			Labels: []string{"Round", "Oval", "Square", none}, Rows: 4, Cursor: 1,
		}, true},
		{"redraw still on question 1 after an answer", strings.Replace(three, "2/3 (2", "1/3 (3", 1), codexui.Overlay{
			Number: 1, Total: 3, Unanswered: 3, Question: "Shape",
			Labels: []string{"Round", "Oval", "Square", none}, Rows: 4, Cursor: 1,
		}, true},
		{"garbage", "› Try \"fix the failing test\"\n  gpt-6 · ~/repo", codexui.Overlay{}, false},
		{"no footer", strings.Split(three, "\n  tab")[0], codexui.Overlay{}, false},
		{"no cursor row", strings.Replace(three, "› 1.", "  1.", 1), codexui.Overlay{}, false},
		{"missing none row", strings.Replace(three, "4. None of the above", "4. Other", 1), codexui.Overlay{}, false},
		{"empty", "", codexui.Overlay{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := codexui.ParseOverlay(strings.Split(tc.screen, "\n"))
			if ok != tc.ok || (ok && !reflect.DeepEqual(got, tc.want)) {
				t.Fatalf("ParseOverlay = %+v, %v; want %+v, %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestParseOverlayNotesOpen(t *testing.T) {
	t.Parallel()
	screen := `  Question 2/3 (2 unanswered)
  Shape
    1. Round              Round shape.
    2. Oval               Oval shape.
    3. Square             Square shape.
  › 4. None of the above  Optionally, add details in notes (tab)
  › Add notes
  tab or esc to clear notes | enter to submit answer | esc to interrupt`
	got, ok := codexui.ParseOverlay(strings.Split(screen, "\n"))
	if !ok || !got.NotesOpen || got.Cursor != 4 || got.Rows != 4 {
		t.Fatalf("ParseOverlay = %+v, %v; want notes open, cursor 4, 4 rows", got, ok)
	}
}
