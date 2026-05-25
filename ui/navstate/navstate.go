package navstate

import (
	"strings"

	"github.com/elentok/gx/ui/nav"
)

type TransitionKind int

const (
	TransitionNone    TransitionKind = iota
	TransitionPushed                 // Open: new ViewState pushed onto stack
	TransitionPopped                 // Back: ViewState popped from stack
	TransitionSwitched               // Switch: tab changed, stack cleared
	TransitionQuit                   // Back on empty stack
	TransitionUpdated                // ViewStateChanged: tab memory updated, no page rebuild
)

type Transition struct {
	Kind        TransitionKind
	ActiveTab   nav.TabID
	ViewState   nav.ViewState // current active view state after transition
	PoppedEntry nav.ViewState // only set on TransitionPopped
	PrevViewState nav.ViewState // only set on TransitionSwitched: state before the switch
}

type State struct {
	activeTab          nav.TabID
	liveTab            nav.TabID
	history            []nav.ViewState
	lastViewStateByTab map[nav.TabID]nav.ViewState
	defaultWorktree    string
}

func NewState(defaultWorktreePath string) State {
	s := State{
		defaultWorktree:    defaultWorktreePath,
		lastViewStateByTab: make(map[nav.TabID]nav.ViewState),
	}
	s.initMissingTabs()
	return s
}

func (s State) ActiveTab() nav.TabID        { return s.activeTab }
func (s State) LiveTab() nav.TabID          { return s.liveTab }
func (s *State) SetLiveTab(t nav.TabID)     { s.liveTab = t }
func (s State) LastViewStateForTab(tab nav.TabID) nav.ViewState {
	return s.lastViewStateByTab[tab]
}

func (s State) Active() nav.ViewState {
	if len(s.history) > 0 {
		return s.history[len(s.history)-1]
	}
	return s.lastViewStateByTab[s.activeTab]
}

// TabViewStateForViewContext resolves the full ViewState for a given context,
// applying defaultWorktree fallback and per-tab memory restoration.
func (s *State) TabViewStateForViewContext(ctx nav.ViewContext) nav.ViewState {
	tabID := ResolveTabID(ctx.Tab)
	tabViewState := nav.ViewState{Tab: tabID}

	switch tabID {
	case nav.TabLog, nav.TabCommit:
		tabViewState.WorktreeRoot = ctx.WorktreeRoot
		tabViewState.Ref = ctx.Ref
		if strings.TrimSpace(tabViewState.WorktreeRoot) == "" {
			tabViewState.WorktreeRoot = s.defaultWorktree
		}
	case nav.TabStatus:
		tabViewState.WorktreeRoot = ctx.WorktreeRoot
		tabViewState.InitialPath = ctx.InitialPath
		if strings.TrimSpace(tabViewState.WorktreeRoot) == "" {
			tabViewState.WorktreeRoot = s.defaultWorktree
		}
	}

	// If context has no specific routing info, restore from per-tab memory.
	if ctx.WorktreeRoot == "" && ctx.Ref == "" && ctx.InitialPath == "" {
		if remembered, ok := s.lastViewStateByTab[tabID]; ok {
			tabViewState.WorktreeRoot = remembered.WorktreeRoot
			tabViewState.Ref = remembered.Ref
			tabViewState.InitialPath = remembered.InitialPath
		}
	}

	// Commit tab requires a ref — default to HEAD when none is available.
	if tabID == nav.TabCommit && tabViewState.Ref == "" {
		tabViewState.Ref = "HEAD"
	}

	return tabViewState
}

func (s *State) Open(vs nav.ViewState) Transition {
	tabVS := s.TabViewStateForViewContext(vs.Context())
	tabVS.FocusSubject = vs.FocusSubject
	tabVS.FilterPath = vs.FilterPath
	tabVS.FilterStartLine = vs.FilterStartLine
	tabVS.FilterEndLine = vs.FilterEndLine

	s.history = append(s.history, tabVS)
	s.activeTab = tabVS.Tab
	s.lastViewStateByTab[tabVS.Tab] = tabVS
	s.initMissingTabs()
	return Transition{
		Kind:      TransitionPushed,
		ActiveTab: s.activeTab,
		ViewState: tabVS,
	}
}

func (s *State) Switch(vs nav.ViewState) Transition {
	prev := s.Active()
	tabVS := s.TabViewStateForViewContext(vs.Context())
	tabVS.FocusSubject = vs.FocusSubject

	s.history = nil
	s.liveTab = tabVS.Tab
	s.activeTab = tabVS.Tab
	s.initMissingTabs()
	s.lastViewStateByTab[tabVS.Tab] = tabVS

	return Transition{
		Kind:          TransitionSwitched,
		ActiveTab:     s.activeTab,
		ViewState:     tabVS,
		PrevViewState: prev,
	}
}

func (s *State) Back() Transition {
	if len(s.history) == 0 {
		return Transition{Kind: TransitionQuit}
	}
	popped := s.history[len(s.history)-1]
	s.history = s.history[:len(s.history)-1]
	if len(s.history) > 0 {
		s.activeTab = s.history[len(s.history)-1].Tab
	} else {
		s.activeTab = s.liveTab
	}
	return Transition{
		Kind:        TransitionPopped,
		ActiveTab:   s.activeTab,
		ViewState:   s.Active(),
		PoppedEntry: popped,
	}
}

func (s *State) ApplyViewStateChanged(vs nav.ViewState) Transition {
	tabVS := s.TabViewStateForViewContext(vs.Context())
	s.initMissingTabs()
	s.lastViewStateByTab[tabVS.Tab] = tabVS
	// If the top of the stack is for the same tab, keep it current.
	if len(s.history) > 0 && s.history[len(s.history)-1].Tab == tabVS.Tab {
		s.history[len(s.history)-1] = tabVS
	}
	return Transition{Kind: TransitionUpdated, ActiveTab: s.activeTab, ViewState: tabVS}
}

func (s *State) SetInitialTab(vs nav.ViewState) {
	tabVS := s.TabViewStateForViewContext(vs.Context())
	s.liveTab = tabVS.Tab
	s.activeTab = tabVS.Tab
	s.initMissingTabs()
	s.lastViewStateByTab[tabVS.Tab] = tabVS
}

func (s *State) initMissingTabs() {
	for _, kind := range []nav.TabID{nav.TabWorktrees, nav.TabLog, nav.TabStatus} {
		if _, ok := s.lastViewStateByTab[kind]; !ok {
			s.lastViewStateByTab[kind] = nav.ViewState{Tab: kind}
		}
	}
}

// ResolveTabID maps unknown tab IDs to the default (TabWorktrees).
func ResolveTabID(kind nav.TabID) nav.TabID {
	switch kind {
	case nav.TabWorktrees, nav.TabLog, nav.TabStatus, nav.TabCommit:
		return kind
	default:
		return nav.TabWorktrees
	}
}

// SameViewContext reports whether two ViewContexts are identical.
func SameViewContext(a, b nav.ViewContext) bool {
	return a == b
}
