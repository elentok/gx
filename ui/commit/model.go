package commit

import (
	"strings"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"

	tea "charm.land/bubbletea/v2"
)

type Settings struct {
	UseNerdFontIcons bool
	InputModalBottom config.InputModalBottom
	EnableNavigation bool
}

type Model struct {
	worktreeRoot string
	ref          string
	settings     Settings

	width        int
	height       int
	ready        bool
	keyPrefix    string
	bodyExpanded bool
	details      git.CommitDetails
	err          error
}

func New(worktreeRoot, ref string) Model {
	return NewWithSettings(worktreeRoot, ref, Settings{UseNerdFontIcons: true})
}

func NewWithSettings(worktreeRoot, ref string, settings Settings) Model {
	m := Model{
		worktreeRoot: worktreeRoot,
		ref:          normalizedRef(ref),
		settings:     settings,
		bodyExpanded: true,
	}
	m.reload()
	return m
}

func (m Model) Init() tea.Cmd { return nil }

func normalizedRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "HEAD"
	}
	return ref
}

func (m *Model) reload() {
	m.details, m.err = git.CommitDetailsForRef(m.worktreeRoot, m.ref)
}
