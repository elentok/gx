package rowfmt

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
)

func TestRenderLiveTicketRow_LaunchingShowsTheStepInTheSuffixSlot(t *testing.T) {
	t.Parallel()
	tk := tickets.Ticket{Identifier: "01", Title: "First"}
	live := LiveTicketState{Running: true, Phase: LivePhaseLaunching, LaunchStep: "creating worktree (1/4)"}
	_, suffix, ok := RenderLiveTicketRow(ui.Icons(false), spinner.New(), tk, live, "")
	if !ok || suffix != "Launching: creating worktree (1/4)" {
		t.Errorf("suffix = %q, ok = %v", suffix, ok)
	}

	live = LiveTicketState{Running: true}
	_, suffix, _ = RenderLiveTicketRow(ui.Icons(false), spinner.New(), tk, live, "")
	if !strings.Contains(ansi.Strip(suffix), "(implementing...)") {
		t.Errorf("running suffix = %q, want implementing", suffix)
	}
}
