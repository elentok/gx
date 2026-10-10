package tickets

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/notify"
)

func loadedModelWithTicket(t *testing.T) Model {
	t.Helper()
	root := t.TempDir()
	writeTicket(t, root, "my-epic", "01-first-ticket.md", "Status: open\n\nBody.\n")
	m := NewModel(root, ui.Settings{}, keys.New(nil))
	m = deliverLoad(t, m)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return updated.(Model)
}

// withLink delivers link with no state, as the shell does on a link change
// before any snapshot.
func withLink(m Model, link ServerLink) (Model, tea.Cmd) {
	next, cmd := m.WithServerState(nil, link)
	return next.(Model), cmd
}

func TestServerLink_ServerKeysDisabledWithReason(t *testing.T) {
	t.Parallel()
	cases := []struct {
		link ServerLink
		want string
	}{
		{ServerLinkDown, "server is down"},
		{ServerLinkReadOnly, "read-only"},
	}
	for _, tc := range cases {
		m, _ := withLink(loadedModelWithTicket(t), tc.link)
		for _, key := range []string{"s", "r", "a", "D", "m"} {
			updated, cmd := m.Update(tea.KeyPressMsg{Code: rune(key[0]), Text: key})
			m = updated.(Model)
			if cmd == nil {
				t.Fatalf("link %v key %q: expected a disabled-with-reason notification", tc.link, key)
			}
			nm, ok := cmd().(notify.NotifyMsg)
			if !ok || !strings.Contains(nm.Message, tc.want) {
				t.Fatalf("link %v key %q: got %#v, want reason containing %q", tc.link, key, nm, tc.want)
			}
		}
	}
}

func TestServerLink_DownKeepsMarkdownRow(t *testing.T) {
	m, _ := withLink(loadedModelWithTicket(t), ServerLinkDown)
	content := m.View().Content
	if !strings.Contains(content, "First ticket") {
		t.Fatalf("markdown row should stay, got:\n%s", content)
	}
}
