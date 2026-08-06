// Package history implements gx's standalone Claude Code session browser
// (`gx claude history`): a two-page bubbletea program (projects ->
// conversations) over ~/.claude/projects, independent of gx's main
// app/nav tab shell since it isn't scoped to the current git worktree.
package history

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/claudehistory"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/filter"
	"github.com/elentok/gx/ui/list"
	"github.com/elentok/gx/ui/notify"
)

// page identifies which of the three pages is active.
type page int

const (
	pageProjects page = iota
	pageConversations
	pageGrep
)

// grepScope selects which directories GrepTranscripts searches.
type grepScope int

const (
	grepScopeProject grepScope = iota
	grepScopeGlobal
)

// ProjectLoader loads the projects list. Production code uses
// claudehistory.ListProjects; tests inject a fixture-returning func so no
// real ~/.claude data or terminal is touched.
type ProjectLoader func(root string) ([]claudehistory.Project, error)

// ConversationLoader loads a project's conversations. Production code uses
// claudehistory.ListConversations; tests inject a fixture-returning func.
type ConversationLoader func(dir string) ([]claudehistory.Conversation, error)

// GrepFunc searches transcripts. Production code uses
// claudehistory.GrepTranscripts; tests inject a fixture-returning func.
type GrepFunc func(query string, dirs []string) ([]claudehistory.GrepResult, error)

// grepDebounce is how long the grep page waits after the query stops
// changing before it fires a search, mirroring blf's grepDebounceMsg.
const grepDebounce = 100 * time.Millisecond

// Model is the root bubbletea model for the history browser.
type Model struct {
	root              string
	loadProjects      ProjectLoader
	loadConversations ConversationLoader
	grepFunc          GrepFunc

	page page
	w, h int

	// terminal is the detected terminal multiplexer/emulator, used by ctrl+r
	// to pick how `claude --resume` is split-launched. Zero value
	// (ui.TerminalPlain) in tests that don't set it, which makes CommandWithSplitBare
	// report "split not supported" rather than launching anything real.
	terminal ui.Terminal

	notify notify.Model

	// projects page
	projects   []claudehistory.Project
	projErr    error
	projList   list.Model
	projFilter filter.Model

	// conversations page
	convProjectDir string
	convProjectCwd string
	conversations  []claudehistory.Conversation
	convErr        error
	convList       list.Model
	convFilter     filter.Model

	// grep page
	grepFromPage     page
	grepScope        grepScope
	grepScopeProjDir string
	grepQuery        string
	grepSeq          int
	grepRunning      bool
	grepResults      []claudehistory.GrepResult
	grepErr          error
	rgNotFound       bool
	grepList         list.Model
	grepFilter       filter.Model
}

// NewModel builds the root model. root is the ~/.claude/projects directory
// (empty defers to ListProjects' own default). loadProjects/loadConversations/
// grepFunc are the data-loading funcs to use; Run wires in the real
// claudehistory funcs, tests inject fixtures.
func NewModel(root string, loadProjects ProjectLoader, loadConversations ConversationLoader, grepFunc GrepFunc) Model {
	return Model{
		root:              root,
		loadProjects:      loadProjects,
		loadConversations: loadConversations,
		grepFunc:          grepFunc,
		notify:            notify.New(false),
		projFilter:        filter.NewModel(),
		convFilter:        filter.NewModel(),
		grepFilter:        filter.NewModel(),
	}
}

type projectsLoadedMsg struct {
	projects []claudehistory.Project
	err      error
}

type conversationsLoadedMsg struct {
	conversations []claudehistory.Conversation
	err           error
}

// grepDebounceMsg fires grepDebounce after the query last changed; a stale
// seq or a query that has since changed again is discarded (see
// handleGrepDebounce), mirroring blf's grepSeq/grepDebounceMsg guard.
type grepDebounceMsg struct {
	seq   int
	query string
}

type grepResultsMsg struct {
	results []claudehistory.GrepResult
	err     error
	seq     int
}

func (m Model) Init() tea.Cmd {
	return m.cmdLoadProjects()
}

func (m Model) cmdLoadProjects() tea.Cmd {
	return func() tea.Msg {
		projects, err := m.loadProjects(m.root)
		return projectsLoadedMsg{projects: projects, err: err}
	}
}

func (m Model) cmdLoadConversations(dir string) tea.Cmd {
	return func() tea.Msg {
		conversations, err := m.loadConversations(dir)
		return conversationsLoadedMsg{conversations: conversations, err: err}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var notifyCmd tea.Cmd
	m.notify, notifyCmd = m.notify.Update(msg)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.projFilter.SetWidth(m.w - 4)
		m.convFilter.SetWidth(m.w - 4)
		m.grepFilter.SetWidth(m.w - 4)
		return m, notifyCmd

	case projectsLoadedMsg:
		m.projects = msg.projects
		m.projErr = msg.err
		m.projList.SetSelected(0, max(len(m.filteredProjects()), 1))
		return m, notifyCmd

	case conversationsLoadedMsg:
		m.conversations = msg.conversations
		m.convErr = msg.err
		m.convList.SetSelected(0, max(len(m.filteredConversations()), 1))
		return m, notifyCmd

	case convExportedMsg:
		return m.handleConvExported(msg, notifyCmd)

	case editorFinishedMsg:
		return m.handleEditorFinished(msg, notifyCmd)

	case resumeFinishedMsg:
		return m.handleResumeFinished(msg, notifyCmd)

	case grepDebounceMsg:
		return m.handleGrepDebounce(msg, notifyCmd)

	case grepResultsMsg:
		return m.handleGrepResults(msg, notifyCmd)

	case tea.KeyPressMsg:
		next, cmd := m.handleKey(msg)
		return next, tea.Batch(notifyCmd, cmd)
	}
	return m, notifyCmd
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.page {
	case pageProjects:
		return m.handleProjectsKey(msg)
	case pageConversations:
		return m.handleConversationsKey(msg)
	case pageGrep:
		return m.handleGrepKey(msg)
	}
	return m, nil
}

func (m Model) listHeight() int {
	h := m.h - 4 // panel frame + footer
	switch m.page {
	case pageProjects:
		if m.projFilter.IsActive() {
			h -= 2
		}
	case pageConversations:
		if m.convFilter.IsActive() {
			h -= 2
		}
	case pageGrep:
		h -= 2 // grep's filter box is always shown
		h -= m.grepPreviewBoxHeight()
	}
	return max(h, 1)
}

// grepPreviewBoxHeight returns the total preview frame height (border
// included) as a third of the available terminal height, clamped so both
// the results list and the preview stay usable on small and large
// terminals. Mirrors blf's grepPreviewBoxHeight.
func (m Model) grepPreviewBoxHeight() int {
	h := m.h / 3
	h = min(max(h, 8), 20)
	if m.h-h < 6 {
		h = max(m.h-6, 3)
	}
	return h
}

func (m Model) View() tea.View {
	var body string
	switch m.page {
	case pageProjects:
		body = m.viewProjects()
	case pageConversations:
		body = m.viewConversations()
	case pageGrep:
		body = m.viewGrep()
	}
	if stack := m.notify.View(); stack != "" {
		body = ui.OverlayTopRightMargin(body, stack, m.w, 1, 1)
	}
	return ui.NewMainView(body)
}
