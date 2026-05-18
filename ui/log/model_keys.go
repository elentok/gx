package log

import (
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/list"
	"github.com/elentok/gx/ui/nav"
	"github.com/elentok/gx/ui/notify"

	tea "charm.land/bubbletea/v2"
)

const (
	bindingHelp       keys.BindingID = "help"
	bindingBack       keys.BindingID = "back"
	bindingDown       keys.BindingID = "down"
	bindingUp         keys.BindingID = "up"
	bindingOpen       keys.BindingID = "open"
	bindingBottom     keys.BindingID = "bottom"
	bindingReload     keys.BindingID = "reload"
	bindingTop        keys.BindingID = "top"
	bindingGotoHead   keys.BindingID = "goto-head"
	bindingGotoWT     keys.BindingID = "goto-worktrees"
	bindingGotoLog    keys.BindingID = "goto-log"
	bindingGotoStatus keys.BindingID = "goto-status"
	bindingNextTag    keys.BindingID = "next-tag"
	bindingPrevTag    keys.BindingID = "prev-tag"
	bindingCancel     keys.BindingID = "cancel-chord"
	bindingAmend      keys.BindingID = "amend"
	bindingBump       keys.BindingID = "bump"
	bindingReword     keys.BindingID = "reword"
	bindingPageDown          keys.BindingID = "page-down"
	bindingPageUp            keys.BindingID = "page-up"
	bindingPull              keys.BindingID = "pull"
	bindingPush              keys.BindingID = "push"
	bindingViewOutput        keys.BindingID = "view-output"
	bindingRebaseInteractive keys.BindingID = "rebase-interactive"
	bindingClearFilter       keys.BindingID = "clear-filter"
)

func newLogManager() keys.Manager {
	return keys.New([]keys.Binding{
		{ID: bindingHelp, Seq: []string{"?"}, Categories: []string{"Other"}, Title: "help"},
		{ID: bindingBack, Seq: []string{"q"}, Categories: []string{"Other"}, Title: "back"},
		{ID: bindingBack, Seq: []string{"esc"}, Categories: []string{}, Title: ""},
		{ID: bindingDown, Seq: []string{"j"}, Categories: []string{"Navigation"}, Title: "down", Display: "↓/j"},
		{ID: bindingDown, Seq: []string{"down"}, Categories: []string{}, Title: ""},
		{ID: bindingUp, Seq: []string{"k"}, Categories: []string{"Navigation"}, Title: "up", Display: "↑/k"},
		{ID: bindingUp, Seq: []string{"up"}, Categories: []string{}, Title: ""},
		{ID: bindingOpen, Seq: []string{"enter"}, Categories: []string{"Navigation"}, Title: "open commit"},
		{ID: bindingBottom, Seq: []string{"G"}, Categories: []string{"Navigation"}, Title: "bottom", Display: "G"},
		{ID: bindingBottom, Seq: []string{"shift+g"}, Categories: []string{}, Title: ""},
		{ID: bindingReload, Seq: []string{"R"}, Categories: []string{"Other"}, Title: "reload"},

		{ID: bindingTop, Seq: []string{"g", "g"}, Categories: []string{"Navigation"}, Title: "top"},
		{ID: bindingViewOutput, Seq: []string{"g", "o"}, Categories: []string{"Other"}, Title: "view output"},
		{ID: bindingGotoHead, Seq: []string{"g", "h"}, Categories: []string{"Jump"}, Title: "goto HEAD"},
		{ID: bindingGotoWT, Seq: []string{"g", "w"}, Categories: []string{"Go to"}, Title: "goto worktrees"},
		{ID: bindingGotoLog, Seq: []string{"g", "l"}, Categories: []string{"Go to"}, Title: "goto log"},
		{ID: bindingGotoStatus, Seq: []string{"g", "s"}, Categories: []string{"Go to"}, Title: "goto status"},
		{ID: bindingCancel, Seq: []string{"g", "esc"}, Categories: []string{}, Title: ""},

		{ID: bindingNextTag, Seq: []string{"]", "t"}, Categories: []string{"Jump"}, Title: "next tag"},
		{ID: bindingCancel, Seq: []string{"]", "esc"}, Categories: []string{}, Title: ""},
		{ID: bindingNextTag, Seq: []string{"shift+]", "t"}, Categories: []string{}, Title: ""},
		{ID: bindingCancel, Seq: []string{"shift+]", "esc"}, Categories: []string{}, Title: ""},
		{ID: bindingPrevTag, Seq: []string{"[", "t"}, Categories: []string{"Jump"}, Title: "prev tag"},
		{ID: bindingCancel, Seq: []string{"[", "esc"}, Categories: []string{}, Title: ""},
		{ID: bindingPrevTag, Seq: []string{"shift+[", "t"}, Categories: []string{}, Title: ""},
		{ID: bindingCancel, Seq: []string{"shift+[", "esc"}, Categories: []string{}, Title: ""},

		{ID: bindingPull, Seq: []string{"p"}, Categories: []string{"Git"}, Title: "pull"},
		{ID: bindingPush, Seq: []string{"P"}, Categories: []string{"Git"}, Title: "push"},
		{ID: bindingAmend, Seq: []string{"A"}, Categories: []string{"Actions"}, Title: "amend commit with staged changes"},
		{ID: bindingBump, Seq: []string{"B"}, Categories: []string{"Actions"}, Title: "bump version"},
		{ID: bindingRebaseInteractive, Seq: []string{"r", "i"}, Categories: []string{"Actions"}, Title: "rebase -i from commit"},
		{ID: bindingReword, Seq: []string{"r", "w"}, Categories: []string{"Actions"}, Title: "reword commit"},
		{ID: bindingCancel, Seq: []string{"r", "esc"}, Categories: []string{}, Title: ""},
		{ID: bindingPageDown, Seq: []string{"ctrl+d"}, Categories: []string{"Navigation"}, Title: "page down"},
		{ID: bindingPageUp, Seq: []string{"ctrl+u"}, Categories: []string{"Navigation"}, Title: "page up"},
		{ID: bindingClearFilter, Seq: []string{"f"}, Categories: []string{"Filter"}, Title: "clear filter"},
	})
}

func (m Model) dispatchBinding(id keys.BindingID) (tea.Model, tea.Cmd) {
	switch id {
	case bindingHelp:
		m.keys.Reset()
		m.help.Open(m.width, m.height)
		return m, nil
	case bindingBack:
		if m.settings.EnableNavigation {
			return m, nav.Back()
		}
		return m, tea.Quit
	case bindingDown:
		m.list.Navigate(1, len(m.rows), maxInt(1, m.height-3))
		return m, nil
	case bindingUp:
		m.list.Navigate(-1, len(m.rows), maxInt(1, m.height-3))
		return m, nil
	case bindingOpen:
		return m, m.openSelected()
	case bindingBottom:
		m.list.SetSelected(len(m.rows)-1, len(m.rows))
		m.list.EnsureSelectionVisible(len(m.rows), maxInt(1, m.height-3))
		return m, nil
	case bindingReload:
		m.refreshing = true
		return m, tea.Batch(notify.Progress("refresh", "refreshing..."), m.cmdReload())
	case bindingTop:
		m.list.SetSelected(0, len(m.rows))
		m.list.EnsureSelectionVisible(len(m.rows), maxInt(1, m.height-3))
		m.statusMsg = ""
		return m, nil
	case bindingGotoHead:
		m.statusMsg = ""
		if m.startRef != "HEAD" {
			return m, nav.Replace(nav.Route{Kind: nav.RouteLog, WorktreeRoot: m.worktreeRoot, Ref: "HEAD"})
		}
		return m, nil
	case bindingGotoWT:
		m.statusMsg = ""
		return m, nav.Replace(nav.Route{Kind: nav.RouteWorktrees})
	case bindingGotoLog:
		m.statusMsg = ""
		return m, nav.Replace(nav.Route{Kind: nav.RouteLog, WorktreeRoot: m.worktreeRoot, Ref: m.startRef})
	case bindingGotoStatus:
		m.statusMsg = ""
		return m, nav.Replace(nav.Route{Kind: nav.RouteStatus, WorktreeRoot: m.worktreeRoot})
	case bindingNextTag:
		m.jumpToTaggedCommit(1)
		return m, nil
	case bindingPrevTag:
		m.jumpToTaggedCommit(-1)
		return m, nil
	case bindingCancel:
		m.statusMsg = ""
		return m, nil
	case bindingViewOutput:
		if m.output.HasContent() {
			m.output.Open(m.width, m.height)
		}
		return m, nil
	case bindingPull:
		cmd := m.pull.Open(m.worktreeRoot)
		return m, cmd
	case bindingPush:
		if err := m.push.Open(m.worktreeRoot); err != nil {
			m.statusMsg = err.Error()
			return m, nil
		}
		return m, nil
	case bindingAmend:
		if err := m.openAmendConfirm(); err != nil {
			m.statusMsg = err.Error()
		}
		return m, nil
	case bindingBump:
		if err := m.bump.Open(m.worktreeRoot); err != nil {
			m.statusMsg = err.Error()
		}
		return m, nil
	case bindingReword:
		return m, m.cmdFetchRewordDetails()
	case bindingPageDown:
		m.list.ScrollPage(list.DefaultScroll, len(m.rows), maxInt(1, m.height-3))
		return m, nil
	case bindingPageUp:
		m.list.ScrollPage(-list.DefaultScroll, len(m.rows), maxInt(1, m.height-3))
		return m, nil
	case bindingRebaseInteractive:
		return m.startRebaseInteractive()
	case bindingClearFilter:
		if !m.filter.IsActive() {
			return m, nil
		}
		m.filter = LogFilter{}
		m.refreshing = true
		return m, tea.Batch(notify.Progress("filter", "clearing filter..."), m.cmdReload())
	}
	return m, nil
}
