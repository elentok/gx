package rowactions

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
)

func TestTranscript_RendersLatestSessionInModal(t *testing.T) {
	prev := LookupSession
	t.Cleanup(func() { LookupSession = prev })
	LookupSession = tickettest.SessionLookup(map[string]string{"old": "# old run", "new": "# latest run\n\nhello from the agent"})
	path := tickettest.WriteTranscriptTicket(t, "session_ids:\n  - old\n  - new\n")

	loaded, ok := CmdTranscript(config.Dirs{}, path, "gx:epic/01", 120)().(WatchLoadedMsg)
	if !ok {
		t.Fatal("expected WatchLoadedMsg")
	}
	m := WatchModal{}.Open(loaded, 120, 40)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "hello from the agent") || strings.Contains(view, "old run") {
		t.Errorf("modal does not show the latest session:\n%s", view)
	}
	if !m.vp.AtBottom() {
		t.Error("modal not scrolled to the bottom")
	}
}
