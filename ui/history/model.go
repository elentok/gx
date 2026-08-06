// Package history implements gx's standalone Claude Code session browser
// (`gx claude history`): a two-page bubbletea program (projects ->
// conversations) over ~/.claude/projects, independent of gx's main
// app/nav tab shell since it isn't scoped to the current git worktree.
package history

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/claudehistory"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/filter"
	"github.com/elentok/gx/ui/list"
)

// page identifies which of the two pages is active.
type page int

const (
	pageProjects page = iota
	pageConversations
)

// ProjectLoader loads the projects list. Production code uses
// claudehistory.ListProjects; tests inject a fixture-returning func so no
// real ~/.claude data or terminal is touched.
type ProjectLoader func(root string) ([]claudehistory.Project, error)

// ConversationLoader loads a project's conversations. Production code uses
// claudehistory.ListConversations; tests inject a fixture-returning func.
type ConversationLoader func(dir string) ([]claudehistory.Conversation, error)

// Model is the root bubbletea model for the history browser.
type Model struct {
	root              string
	loadProjects      ProjectLoader
	loadConversations ConversationLoader

	page page
	w, h int

	// projects page
	projects   []claudehistory.Project
	projErr    error
	projList   list.Model
	projFilter filter.Model

	// conversations page
	convProjectDir string
	conversations  []claudehistory.Conversation
	convErr        error
	convList       list.Model
	convFilter     filter.Model
}

// NewModel builds the root model. root is the ~/.claude/projects directory
// (empty defers to ListProjects' own default). loadProjects/loadConversations
// are the data-loading funcs to use; Run wires in the real claudehistory
// funcs, tests inject fixtures.
func NewModel(root string, loadProjects ProjectLoader, loadConversations ConversationLoader) Model {
	return Model{
		root:              root,
		loadProjects:      loadProjects,
		loadConversations: loadConversations,
		projFilter:        filter.NewModel(),
		convFilter:        filter.NewModel(),
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
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.projFilter.SetWidth(m.w - 4)
		m.convFilter.SetWidth(m.w - 4)
		return m, nil

	case projectsLoadedMsg:
		m.projects = msg.projects
		m.projErr = msg.err
		m.projList.SetSelected(0, max(len(m.filteredProjects()), 1))
		return m, nil

	case conversationsLoadedMsg:
		m.conversations = msg.conversations
		m.convErr = msg.err
		m.convList.SetSelected(0, max(len(m.filteredConversations()), 1))
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.page {
	case pageProjects:
		return m.handleProjectsKey(msg)
	case pageConversations:
		return m.handleConversationsKey(msg)
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
	}
	return max(h, 1)
}

func (m Model) View() tea.View {
	var body string
	switch m.page {
	case pageProjects:
		body = m.viewProjects()
	case pageConversations:
		body = m.viewConversations()
	}
	return ui.NewMainView(body)
}
