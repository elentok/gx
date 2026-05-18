package commit

import (
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/filetree"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/search"

	"github.com/elentok/gx/ui/amend"
	"github.com/elentok/gx/ui/help"
	"github.com/elentok/gx/ui/reword"

	tea "charm.land/bubbletea/v2"
)

type Model struct {
	worktreeRoot string
	ref          string
	settings     ui.Settings
	filterPath   string

	width        int
	height       int
	ready        bool
	focusHeader  bool
	focusDiff    bool
	bodyExpanded bool
	details      git.CommitDetails
	headerOffset int
	err          error

	commitDiffArea
	commitSearchState
	commitSidebarState

	help help.Model
	keys keys.Manager

	amendConfirm amend.Model

	reword           reword.Model
	rewordTmpFile    string
	rewordOrigMsg    string
	rewordNewSubject string
}

type editCommentFinishedMsg struct {
	err      error
	splitApp string
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

func NewModel(worktreeRoot, ref, filterPath string, settings ui.Settings, extraKeys keys.Manager) Model {
	m := Model{
		worktreeRoot: worktreeRoot,
		ref:          normalizedRef(ref),
		settings:     settings,
		filterPath:   strings.TrimSpace(filterPath),
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
		keys: newCommitManager(),
	}
	m.help = help.NewModel(help.BuildSections(m.keys, extraKeys))
	m.amendConfirm = amend.New()
	m.reword = reword.New()
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

func (m *Model) KeyManager() *keys.Manager {
	return &m.keys
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
	m.applyFilterPathSearch()
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

func (m *Model) applyFilterPathSearch() {
	if m.filterPath == "" {
		return
	}
	m.searchScope = searchScopeSidebar
	matches := m.computeSearchMatches(m.filterPath)
	m.search.SetPassiveResults(m.filterPath, matches)
	m.jumpToCurrentMatch()
}
