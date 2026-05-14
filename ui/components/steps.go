package components

import (
	"strings"

	"github.com/elentok/gx/ui"
)

// Step represents one unit of work in a multi-step operation.
type Step struct {
	TitleBefore  string // shown while pending    e.g. "rebase"
	RunningTitle string // shown while running    e.g. "rebasing..."
	TitleAfter   string // shown when done        e.g. "rebased"
	TitleFailed  string // shown when failed      e.g. "rebase failed"
	IsRunning    bool
	IsDone       bool
	HasFailed    bool
}

var (
	stepPendingStyle = ui.StyleMuted
	stepRunningStyle = ui.StyleBody
	stepDoneStyle    = ui.StyleBody
	stepFailedStyle  = ui.StyleWarning
)

// RenderSteps renders a vertical list of steps with status icons.
func RenderSteps(steps []Step, spinnerFrame string) string {
	lines := make([]string, 0, len(steps))
	for _, s := range steps {
		lines = append(lines, renderStep(s, spinnerFrame))
	}
	return strings.Join(lines, "\n")
}

func renderStep(s Step, spinnerFrame string) string {
	switch {
	case s.HasFailed:
		icon := stepFailedStyle.Render("✗")
		title := stepFailedStyle.Render(s.TitleFailed)
		return icon + " " + title
	case s.IsDone:
		icon := stepDoneStyle.Render("✓")
		title := stepDoneStyle.Render(s.TitleAfter)
		return icon + " " + title
	case s.IsRunning:
		icon := stepRunningStyle.Render(spinnerFrame)
		title := stepRunningStyle.Render(s.RunningTitle)
		return icon + " " + title
	default:
		icon := stepPendingStyle.Render("○")
		title := stepPendingStyle.Render(s.TitleBefore)
		return icon + " " + title
	}
}
