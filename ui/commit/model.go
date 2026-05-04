package commit

import (
	"strings"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/explorer"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
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

	width         int
	height        int
	ready         bool
	focusDiff     bool
	diffNavMode   explorer.NavMode
	wrapSoft      bool
	keyPrefix     string
	bodyExpanded  bool
	details       git.CommitDetails
	files         []git.CommitFile
	fileEntries   []commitFileEntry
	collapsedDirs map[string]bool
	selected      int
	section       explorer.SectionData
	diffViewport  viewport.Model
	searchMode    commitSearchMode
	searchScope   commitSearchScope
	searchQuery   string
	searchMatches []explorer.DiffSearchMatch
	fileMatches   []int
	searchCursor  int
	searchInput   textinput.Model
	statusMsg     string
	err           error
}

func New(worktreeRoot, ref string) Model {
	return NewWithSettings(worktreeRoot, ref, Settings{UseNerdFontIcons: true})
}

func NewWithSettings(worktreeRoot, ref string, settings Settings) Model {
	m := Model{
		worktreeRoot:  worktreeRoot,
		ref:           normalizedRef(ref),
		settings:      settings,
		bodyExpanded:  true,
		diffNavMode:   explorer.NavHunk,
		wrapSoft:      true,
		diffViewport:  viewport.New(),
		collapsedDirs: map[string]bool{},
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
	m.fileEntries = buildCommitFileEntries(m.files, m.collapsedDirs)
	if m.selected >= len(m.fileEntries) {
		m.selected = len(m.fileEntries) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
	if entry, ok := m.selectedCommitEntry(); !ok || entry.Kind != commitFileEntryFile {
		m.selectFirstCommitFile()
	}
	m.refreshDiff()
}

func (m *Model) refreshDiff() {
	file, ok := m.selectedCommitFile()
	if !ok {
		m.section = explorer.NewSectionData()
		return
	}
	rawDiff, err := git.CommitFileDiffForRef(m.worktreeRoot, m.ref, file.Path)
	if err != nil {
		m.err = err
		m.section = explorer.NewSectionData()
		return
	}
	colorDiff, err := git.CommitFileDiffWithDeltaForRef(m.worktreeRoot, m.ref, file.Path, m.currentDiffRenderWidth())
	if err != nil {
		colorDiff = rawDiff
	}
	m.section = explorer.BuildSectionData(rawDiff, colorDiff, m.section, false)
	if strings.TrimSpace(m.searchQuery) != "" && m.searchScope == searchScopeDiff {
		cursor := m.searchCursor
		m.recomputeSearchMatches()
		if len(m.searchMatches) == 0 {
			m.searchCursor = 0
		} else if cursor < len(m.searchMatches) {
			m.searchCursor = cursor
		} else {
			m.searchCursor = len(m.searchMatches) - 1
		}
	}
	m.syncDiffViewport()
}

func (m *Model) selectFirstCommitFile() {
	for i, entry := range m.fileEntries {
		if entry.Kind == commitFileEntryFile {
			m.selected = i
			return
		}
	}
}
