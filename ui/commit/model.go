package commit

import (
	"strings"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/filetree"
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
	diffModel diffview.Model
}

type commitSearchState struct {
	search      search.Model
	searchScope commitSearchScope
}

type commitSidebarState struct {
	files         []git.CommitFile
	fileTreeModel filetree.Model[git.CommitFile]
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
			diffModel: diffview.NewModel(),
		},
		commitSearchState: commitSearchState{
			search: search.NewModel(),
		},
		commitSidebarState: commitSidebarState{
			fileTreeModel: filetree.NewModel[git.CommitFile](),
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
		m.diffModel.SetData(diffview.NewDiffData())
		m.headerOffset = 0
		return
	}
	m.files, m.err = git.CommitFilesForRef(m.worktreeRoot, m.ref)
	if m.err != nil {
		m.diffModel.SetData(diffview.NewDiffData())
		m.headerOffset = 0
		return
	}
	entries := filetree.BuildEntriesFromValues(
		m.files,
		func(file git.CommitFile) string { return file.Path },
		m.fileTreeModel.CollapsedDirs(),
	)
	m.fileTreeModel.SetEntries(entries)
	if entry, ok := m.selectedCommitEntry(); !ok || entry.Kind != filetree.EntryFile {
		m.selectFirstCommitFile()
	}
	m.headerOffset = 0
	m.refreshDiff()
}

func (m *Model) refreshDiff() {
	file, ok := m.selectedCommitFile()
	if !ok {
		m.diffModel.SetData(diffview.NewDiffData())
		return
	}
	rawDiff, err := git.CommitFileDiffForRef(m.worktreeRoot, m.ref, file.Path)
	if err != nil {
		m.err = err
		m.diffModel.SetData(diffview.NewDiffData())
		return
	}
	colorDiff, err := git.CommitFileDiffWithDeltaForRef(m.worktreeRoot, m.ref, file.Path, m.currentDiffRenderWidth())
	if err != nil {
		colorDiff = rawDiff
	}
	m.diffModel.BuildFromRaw(rawDiff, colorDiff)
	if m.search.HasQuery() && m.searchScope == searchScopeDiff {
		matches := m.computeSearchMatches(m.search.Query())
		m.search.SetMatches(matches)
	}
	m.syncDiffViewport()
}

func (m *Model) selectFirstCommitFile() {
	entries := m.fileTreeModel.Entries()
	for i, entry := range entries {
		if entry.Kind == filetree.EntryFile {
			m.fileTreeModel.SetSelectedIndex(i)
			return
		}
	}
}
