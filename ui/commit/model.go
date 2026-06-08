package commit

import (
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/filetree"
	"github.com/elentok/gx/ui/imagediff"
	"github.com/elentok/gx/ui/keys"

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
	inactive     bool
	focusHeader  bool
	focusDiff    bool
	bodyExpanded bool
	details      git.CommitDetails
	headerOffset int
	err          error

	commitDiffArea
	commitSidebarState

	help help.Model
	keys keys.Manager

	amendConfirm amend.Model

	reword reword.Model

	// Inline image-diff overlay (ADR 0010). The detail panel is composed into a
	// split view and never learns its absolute screen position from
	// lipgloss.Join*, so the container injects it via WithScreenOrigin.
	overlay             imagediff.Overlay
	screenCol           int
	screenRow           int
	screenVisible       bool
	fetchImageDiffBlobs func(ref string, file git.CommitFile) (old, newBytes []byte, oldOK, newOK bool)
}

type editCommentFinishedMsg struct {
	err      error
	splitApp string
}

type commitDiffArea struct {
	diffModel        diffview.Model
	diffContextLines int
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
			diffModel:        diffview.NewModel(settings.UseNerdFontIcons),
			diffContextLines: settings.DiffContextLines,
		},
		commitSidebarState: commitSidebarState{
			fileTreeModel: filetree.NewModel[git.CommitFile](),
		},
		keys: newCommitManager(),
		overlay: imagediff.NewOverlay(
			imagediff.WriteToStdout, imagediff.DefaultDetectCapability),
		fetchImageDiffBlobs: func(ref string, file git.CommitFile) (old, newBytes []byte, oldOK, newOK bool) {
			return git.CommitImageDiffBlobs(worktreeRoot, ref, file)
		},
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

func (m Model) KeyManager() keys.Manager {
	return m.keys
}

// WithRef loads the given ref and returns an updated model plus the image-diff
// disrupt command. Used by the log/stash split views to swap the displayed
// commit when list selection changes. Selection moves are keys routed to the
// list (never into commit.Update), so the setter returns the disrupt cmd for the
// container to batch (ADR 0010).
func (m Model) WithRef(ref string) (Model, tea.Cmd) {
	m.ref = normalizedRef(ref)
	m.reload()
	var cmd tea.Cmd
	m.overlay, cmd = m.overlay.Disrupt(m.settings.ImageDiffs)
	return m, cmd
}

// WithScreenOrigin injects the detail panel's absolute screen origin (and
// whether it is currently visible), computed by the split container. Changing it
// is a disrupting event for any active image-diff overlay, so it returns the
// disrupt cmd for the container to batch. A no-op call (unchanged origin) returns
// no command so resizes that don't move the panel don't thrash placements.
func (m Model) WithScreenOrigin(col, row int, visible bool) (Model, tea.Cmd) {
	if m.screenCol == col && m.screenRow == row && m.screenVisible == visible {
		return m, nil
	}
	m.screenCol, m.screenRow, m.screenVisible = col, row, visible
	var cmd tea.Cmd
	m.overlay, cmd = m.overlay.Disrupt(m.settings.ImageDiffs)
	return m, cmd
}

// WithContainerFocus returns a copy rendered as active only when its containing
// split/detail panel owns keyboard focus.
func (m Model) WithContainerFocus(focused bool) Model {
	m.inactive = !focused
	return m
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
	rawDiff, err := git.CommitFileDiffForRef(m.worktreeRoot, m.ref, file.Path, m.currentDiffContextLines())
	if err != nil {
		m.err = err
		m.diffModel.SetData(diffview.NewDiffData())
		return
	}
	sideBySide := m.diffModel.RenderMode() == diffview.RenderModeSideBySide
	colorDiff, err := git.CommitFileDiffWithDeltaForRef(m.worktreeRoot, m.ref, file.Path, m.currentDiffContextLines(), m.currentDiffRenderWidth(), sideBySide)
	if err != nil {
		colorDiff = rawDiff
	}
	m.diffModel.BuildFromRaw(rawDiff, colorDiff)
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
	m.fileTreeModel.ApplyPassiveSearch(m.filterPath, m.fileEntrySearchText)
	if m.fileTreeModel.FocusCurrentSearchMatch() {
		m.refreshDiff()
	}
}
