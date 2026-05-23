package app

import (
	"strings"

	"github.com/elentok/gx/ui/nav"
)

type routerTabState struct {
	tabID        nav.TabID
	worktreeRoot string
	ref          string
	initialPath  string
}

type routerState struct {
	activeTab nav.TabID
	tabs      map[nav.TabID]routerTabState
	histories map[nav.TabID][]nav.ViewState
	history   []nav.ViewState
}

func newRouterState(initialRoute nav.ViewState, activeWorktreePath string) routerState {
	if initialRoute.Tab == "" {
		initialRoute = nav.ViewState{Tab: nav.TabWorktrees}
	}
	r := routerState{
		activeTab: tabForRoute(initialRoute.Tab),
		tabs:      make(map[nav.TabID]routerTabState),
		histories: make(map[nav.TabID][]nav.ViewState),
	}
	r.ensureTabs()
	r.tabs[r.activeTab] = tabContextForViewContext(initialRoute.Context(), activeWorktreePath)
	if initialRoute.Tab == nav.TabCommit {
		r.history = append(r.history, initialRoute)
		r.histories[r.activeTab] = append([]nav.ViewState(nil), r.history...)
	}
	return r
}

func (r *routerState) ensureTabs() {
	for _, tabID := range []nav.TabID{nav.TabWorktrees, nav.TabLog, nav.TabStatus} {
		if _, ok := r.tabs[tabID]; ok {
			continue
		}
		r.tabs[tabID] = routerTabState{tabID: tabID}
	}
}

func (r *routerState) replace(viewState nav.ViewState, activeWorktreePath string) {
	next := tabContextForViewContext(viewState.Context(), activeWorktreePath)
	r.ensureTabs()
	r.histories[r.activeTab] = append([]nav.ViewState(nil), r.history...)
	r.activeTab = next.tabID
	r.history = append([]nav.ViewState(nil), r.histories[r.activeTab]...)
	current := r.tabs[next.tabID]

	if !sameRouterTabState(current, next) {
		r.history = nil
		r.histories[r.activeTab] = nil
		r.tabs[next.tabID] = next
		return
	}
}

func (r *routerState) viewStateChanged(viewState nav.ViewState, activeWorktreePath string) {
	next := tabContextForViewContext(viewState.Context(), activeWorktreePath)
	r.ensureTabs()
	r.tabs[next.tabID] = next
}

func (r *routerState) push(viewState nav.ViewState) {
	r.history = append(r.history, viewState)
	r.histories[r.activeTab] = append([]nav.ViewState(nil), r.history...)
}

func (r *routerState) back() (nav.ViewState, bool) {
	if len(r.history) == 0 {
		return nav.ViewState{}, false
	}
	popped := r.history[len(r.history)-1]
	r.history = r.history[:len(r.history)-1]
	r.histories[r.activeTab] = append([]nav.ViewState(nil), r.history...)
	return popped, true
}

func tabContextForViewContext(ctx nav.ViewContext, activeWorktreePath string) routerTabState {
	tab := routerTabState{tabID: tabForRoute(ctx.Tab)}
	switch tab.tabID {
	case nav.TabLog, nav.TabCommit:
		tab.ref = ctx.Ref
		tab.worktreeRoot = ctx.WorktreeRoot
		if strings.TrimSpace(tab.worktreeRoot) == "" {
			tab.worktreeRoot = activeWorktreePath
		}
	case nav.TabStatus:
		tab.initialPath = ctx.InitialPath
		tab.worktreeRoot = ctx.WorktreeRoot
		if strings.TrimSpace(tab.worktreeRoot) == "" {
			tab.worktreeRoot = activeWorktreePath
		}
	}
	return tab
}

func sameRouterTabState(a, b routerTabState) bool {
	return a.tabID == b.tabID &&
		a.worktreeRoot == b.worktreeRoot &&
		a.ref == b.ref &&
		a.initialPath == b.initialPath
}
