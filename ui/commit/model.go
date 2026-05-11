package commit

import (
	"strings"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/search"

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

	width        int
	height       int
	ready        bool
	focusHeader  bool
	focusDiff    bool
	keyPrefix    string
	bodyExpanded bool
	details      git.CommitDetails
	statusMsg    string
	headerOffset int
	err          error

	commitDiffArea
	commitSearchState
	commitSidebarState

	helpOpen     bool
	helpViewport viewport.Model
}

type commitDiffArea struct {
	diffNavMode  diffview.NavMode
	wrapSoft     bool
	section      diffview.DiffBuffer
	diffViewport viewport.Model
}

type commitSearchState struct {
	search      search.Model
	searchScope commitSearchScope
}

type commitSidebarState struct {
	files         []git.CommitFile
	fileEntries   []commitFileEntry
	collapsedDirs map[string]bool
	selected      int
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
		commitDiffArea: commitDiffArea{
			diffNavMode:  diffview.NavModeHunk,
			wrapSoft:     true,
			diffViewport: viewport.New(),
		},
		commitSearchState: commitSearchState{
			search: search.NewModel(),
		},
		commitSidebarState: commitSidebarState{
			collapsedDirs: map[string]bool{},
		},
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
		m.section = diffview.NewDiffBuffer()
		m.headerOffset = 0
		return
	}
	m.files, m.err = git.CommitFilesForRef(m.worktreeRoot, m.ref)
	if m.err != nil {
		m.section = diffview.NewDiffBuffer()
		m.headerOffset = 0
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
	m.headerOffset = 0
	m.refreshDiff()
}

func (m *Model) refreshDiff() {
	file, ok := m.selectedCommitFile()
	if !ok {
		m.section = diffview.NewDiffBuffer()
		return
	}
	rawDiff, err := git.CommitFileDiffForRef(m.worktreeRoot, m.ref, file.Path)
	if err != nil {
		m.err = err
		m.section = diffview.NewDiffBuffer()
		return
	}
	colorDiff, err := git.CommitFileDiffWithDeltaForRef(m.worktreeRoot, m.ref, file.Path, m.currentDiffRenderWidth())
	if err != nil {
		colorDiff = rawDiff
	}
	m.section = diffview.BuildDiffBuffer(rawDiff, colorDiff, m.section, false)
	if m.search.HasQuery() && m.searchScope == searchScopeDiff {
		matches := m.computeSearchMatches(m.search.Query())
		m.search.SetMatches(matches)
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
