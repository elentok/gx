// Package serverlink is the Tickets tab's link to the server: the client, the
// last delivered snapshot and the project scope applied to it.
package serverlink

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/tickets/tabapi"
	"github.com/elentok/gx/viewmodel"
)

// Binding holds what the tab knows of the server.
type Binding struct {
	// API feeds VM; nil keeps the tab reading the store for good.
	API tabapi.ServerAPI
	// Link is the link half of the last Receive; nothing else writes it.
	Link tabapi.ServerLink
	// Older: the last snapshot came from a server without rendered status; the
	// tab shows tabapi.OlderServerNotice and no tickets.
	Older bool
	VM    viewmodel.State
	// Scope is which projects the tab shows; owned by the tab, so no snapshot
	// or event can reset it.
	Scope viewmodel.Scope
}

// ReadsDisk reports whether the tab reads the store itself: the server is down
// or there is no client at all. The shell's state is then stale and ignored.
func (b Binding) ReadsDisk() bool {
	return b.API == nil || b.Link == tabapi.ServerLinkDown
}

// Receive takes a state delivery. apply reports that VM changed and the rows
// need rebuilding; loadDisk that the link just crossed into down, so the tab
// must load from the store.
func (b *Binding) Receive(st *viewmodel.State, link tabapi.ServerLink) (apply, loadDisk bool) {
	wasDown := b.Link == tabapi.ServerLinkDown
	b.Link = link
	if b.ReadsDisk() {
		b.Older = false
		return false, link == tabapi.ServerLinkDown && !wasDown
	}
	if st == nil {
		return false, false
	}
	b.VM = *st
	b.Older = st.OlderServer()
	if b.Older {
		b.VM = viewmodel.State{}
	}
	return true, false
}

// Epics is the snapshot's epics inside the project scope.
func (b Binding) Epics() []tickets.Epic {
	return tabapi.EpicsFromViewModel(b.VM, b.Scope)
}

// ToggleScope flips between the cwd project and all projects. ok is false when
// the cwd is no registered project; msg is the toast text either way.
func (b *Binding) ToggleScope() (msg string, ok bool) {
	if b.Scope.CwdProject == "" {
		return b.Scope.UnregisteredHint(), false
	}
	b.Scope = b.Scope.Toggle()
	if b.Scope.AllProjects {
		return "all projects", true
	}
	return "project: " + b.Scope.CwdProject, true
}

// DisabledToast is the toast for a key that needs a writable server; blocked
// is false while the server accepts writes.
func (b Binding) DisabledToast() (cmd tea.Cmd, blocked bool) {
	reason := b.Link.DisabledReason()
	if reason == "" {
		return nil, false
	}
	return notify.Info("disabled: " + reason), true
}
