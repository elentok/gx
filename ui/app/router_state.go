package app

import (
	"strings"

	"github.com/elentok/gx/ui/nav"
)

type routerTabState struct {
	kind         nav.RouteKind
	worktreeRoot string
	ref          string
	initialPath  string
}

type routerState struct {
	activeTab nav.RouteKind
	tabs      map[nav.RouteKind]routerTabState
	histories map[nav.RouteKind][]nav.Route
	history   []nav.Route
}

func newRouterState(initialRoute nav.Route, activeWorktreePath string) routerState {
	if initialRoute.Kind == "" {
		initialRoute = nav.Route{Kind: nav.RouteWorktrees}
	}
	r := routerState{
		activeTab: tabForRoute(initialRoute.Kind),
		tabs:      make(map[nav.RouteKind]routerTabState),
		histories: make(map[nav.RouteKind][]nav.Route),
	}
	r.ensureTabs()
	r.tabs[r.activeTab] = routerTabStateForRoute(initialRoute, activeWorktreePath)
	if initialRoute.Kind == nav.RouteCommit {
		r.history = append(r.history, initialRoute)
		r.histories[r.activeTab] = append([]nav.Route(nil), r.history...)
	}
	return r
}

func (r *routerState) ensureTabs() {
	for _, kind := range []nav.RouteKind{nav.RouteWorktrees, nav.RouteLog, nav.RouteStatus} {
		if _, ok := r.tabs[kind]; ok {
			continue
		}
		r.tabs[kind] = routerTabState{kind: kind}
	}
}

func (r *routerState) replace(route nav.Route, activeWorktreePath string) {
	next := routerTabStateForRoute(route, activeWorktreePath)
	r.ensureTabs()
	r.histories[r.activeTab] = append([]nav.Route(nil), r.history...)
	r.activeTab = next.kind
	r.history = append([]nav.Route(nil), r.histories[r.activeTab]...)
	current := r.tabs[next.kind]

	if !sameRouterTabState(current, next) {
		r.history = nil
		r.histories[r.activeTab] = nil
		r.tabs[next.kind] = next
		return
	}
}

func (r *routerState) push(route nav.Route) {
	r.history = append(r.history, route)
	r.histories[r.activeTab] = append([]nav.Route(nil), r.history...)
}

func (r *routerState) back() (nav.Route, bool) {
	if len(r.history) == 0 {
		return nav.Route{}, false
	}
	popped := r.history[len(r.history)-1]
	r.history = r.history[:len(r.history)-1]
	r.histories[r.activeTab] = append([]nav.Route(nil), r.history...)
	return popped, true
}

func routerTabStateForRoute(route nav.Route, activeWorktreePath string) routerTabState {
	tab := routerTabState{kind: tabForRoute(route.Kind)}
	switch tab.kind {
	case nav.RouteLog:
		tab.ref = route.Ref
		tab.worktreeRoot = route.WorktreeRoot
		if strings.TrimSpace(tab.worktreeRoot) == "" {
			tab.worktreeRoot = activeWorktreePath
		}
	case nav.RouteStatus:
		tab.initialPath = route.InitialPath
		tab.worktreeRoot = route.WorktreeRoot
		if strings.TrimSpace(tab.worktreeRoot) == "" {
			tab.worktreeRoot = activeWorktreePath
		}
	}
	return tab
}

func sameRouterTabState(a, b routerTabState) bool {
	return a.kind == b.kind &&
		a.worktreeRoot == b.worktreeRoot &&
		a.ref == b.ref &&
		a.initialPath == b.initialPath
}
