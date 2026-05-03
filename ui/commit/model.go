package commit

import (
	"strings"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/explorer"

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
	focusDiff    bool
	keyPrefix    string
	bodyExpanded bool
	details      git.CommitDetails
	files        []git.CommitFile
	selected     int
	section      explorer.SectionData
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
	if m.err != nil {
		m.files = nil
		m.section = explorer.NewSectionData()
		return
	}
	m.files, m.err = git.CommitFilesForRef(m.worktreeRoot, m.ref)
	if m.err != nil {
		m.section = explorer.NewSectionData()
		return
	}
	if m.selected >= len(m.files) {
		m.selected = len(m.files) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
	m.refreshDiff()
}

func (m *Model) refreshDiff() {
	if len(m.files) == 0 {
		m.section = explorer.NewSectionData()
		return
	}
	file := m.files[m.selected]
	rawDiff, err := git.CommitFileDiffForRef(m.worktreeRoot, m.ref, file.Path)
	if err != nil {
		m.err = err
		m.section = explorer.NewSectionData()
		return
	}
	m.section = explorer.BuildSectionData(rawDiff, rawDiff, m.section, false)
}
