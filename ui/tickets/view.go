package tickets

import (
	"github.com/elentok/gx/server/wire"

	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/tickets/preview"
	"github.com/elentok/gx/ui/tickets/sidebar"
)

var sessionMarkerStyle = lipgloss.NewStyle().Foreground(ui.ColorTeal)

// sidebarEnvFor snapshots what the sidebar reads from the rest of the tab.
func sidebarEnvFor(m Model) sidebar.Env {
	return sidebar.Env{
		Icons:       m.icons(),
		Active:      m.preview.Focus == preview.FocusSidebar,
		Loaded:      m.loaded,
		OlderServer: m.server.Older,
		SessionMarker: func(address string) string {
			for _, s := range m.server.VM.Sessions {
				if s.Ticket == address && s.Status.State != wire.SessionExited {
					return " " + sessionMarkerStyle.Render(m.icons().SessionLive)
				}
			}
			return ""
		},
		HasSuggestedActions: func(epic tickets.Epic, ticket tickets.Ticket, archived bool) bool {
			return m.menus.HasSuggestedActions(m.menuEnv(), epic, ticket, archived)
		},
		PendingText: func(ticketPath string) (string, bool) {
			p, ok := m.server.VM.PendingRowFor(ticketPath)
			if !ok {
				return "", false
			}
			label := p.Verdict
			if label == wire.VerdictConcurrencyCap {
				label = "Waiting for a slot"
			}
			if p.Reason != "" {
				return label + ": " + p.Reason, true
			}
			return label, true
		},
	}
}

func (m Model) icons() ui.IconSet {
	return ui.Icons(m.settings.UseNerdFontIcons)
}
