package agents

import (
	"fmt"
	"strings"
	"time"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/ui/agents/agentstyle"
)

// lastScreen is an Exited session's saved last screen; changedAt tells a
// later exit of the same session (after a resume) from the one loaded.
type lastScreen struct {
	id        string
	changedAt time.Time
	rows      []string
}

type lastScreenMsg lastScreen

// infoLines is the info row then a blank row, or none without an info row.
func infoLines(s wire.SessionInfo, now time.Time) []string {
	if info := infoRow(s, now); info != "" {
		return []string{info, ""}
	}
	return nil
}

// infoRow is the Actionable reason with the prompts `<leader> a` would run,
// else the status detail. A running auto-advance countdown ends it, with the
// way to cancel.
func infoRow(s wire.SessionInfo, now time.Time) string {
	if s.Actionable.Reason != "" {
		row := agentstyle.Reason.Render(s.Actionable.Reason)
		if len(s.Actionable.Action) > 0 {
			row += agentstyle.Action.Render("  <leader> a: " + strings.Join(s.Actionable.Action, " → "))
		}
		if s.AutoAdvanceAt != nil {
			left := max(s.AutoAdvanceAt.Sub(now), 0)
			row += agentstyle.Detail.Render(fmt.Sprintf("  Continuing in %ds", int((left+time.Second-1)/time.Second)))
			row += agentstyle.Action.Render("  Cancel: session menu (m)")
		}
		return row
	}
	if d := agentstyle.StatusDetail(s); d != "" {
		return agentstyle.Detail.Render(d)
	}
	return ""
}
