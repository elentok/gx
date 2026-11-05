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
		Query:       m.search.Query(),
		HasQuery:    m.search.HasQuery(),
		Icons:       m.icons(),
		Active:      m.preview.Focus == preview.FocusSidebar,
		Loaded:      m.loaded,
		OlderServer: m.server.older,
		SessionMarker: func(address string) string {
			for _, s := range m.server.vm.Sessions {
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
			p, ok := m.server.vm.PendingRowFor(ticketPath)
			if !ok {
				return "", false
			}
			if p.Reason != "" {
				return p.Verdict + ": " + p.Reason, true
			}
			return p.Verdict, true
		},
	}
}

func (m Model) icons() ui.IconSet {
	return ui.Icons(m.settings.UseNerdFontIcons)
}
