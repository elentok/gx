package tickets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/confirm"
)

const answerTicket = "# A\n\n## Needs Answer\n\nWhich approach?\n\n## Notes\n\nkeep\n"

func writeAnswerTicket(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "01-a.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestNeedsAnswerSection_StopsAtNextHeading(t *testing.T) {
	t.Parallel()
	got := needsAnswerSection(answerTicket)
	if !strings.Contains(got, "Which approach?") || strings.Contains(got, "keep") {
		t.Errorf("section = %q", got)
	}
	if line := needsAnswerLine(answerTicket); line != 3 {
		t.Errorf("line = %d, want 3", line)
	}
}

func TestHandleAnswerEditorFinished_InPlace(t *testing.T) {
	tests := []struct {
		name       string
		after      string
		wantPrompt string
	}{
		{"answered", strings.Replace(answerTicket, "Which approach?", "Which approach?\n\nA.", 1), "Resume the ticket?"},
		{"unanswered", answerTicket, "No answer found… Keep editing?"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeAnswerTicket(t, tt.after)
			msg := answerEditorFinishedMsg{path: path, before: needsAnswerSection(answerTicket)}
			c, _ := handleAnswerEditorFinished(confirm.New(), msg, func() tea.Msg { return nil })
			if !c.IsOpen || !strings.Contains(c.View(80), tt.wantPrompt) {
				t.Errorf("open=%v view=%q, want prompt %q", c.IsOpen, c.View(80), tt.wantPrompt)
			}
		})
	}
}

func TestHandleAnswerEditorFinished_Split_HintNoModal(t *testing.T) {
	t.Parallel()
	msg := answerEditorFinishedMsg{splitApp: "tmux", path: writeAnswerTicket(t, answerTicket)}
	c, cmd := handleAnswerEditorFinished(confirm.New(), msg, func() tea.Msg { return nil })
	if c.IsOpen {
		t.Error("confirm opened for a split launch, want a hint only")
	}
	if cmd == nil {
		t.Fatal("cmd = nil, want a hint notification")
	}
}

func TestHighlightParkSection_NeedsAnswer_RendersHintUnderHeading(t *testing.T) {
	t.Parallel()
	out, target, ok := highlightParkSection("## Needs Answer\nWhich?\n", tickets.StatusNeedsAnswer)
	lines := strings.Split(out, "\n")
	if !ok || target != 0 || !strings.Contains(lines[1], needsAnswerHint) {
		t.Errorf("ok=%v target=%d lines=%q, want hint right under the heading", ok, target, lines)
	}
}
