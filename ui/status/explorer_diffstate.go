package status

import (
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/diff"
	"github.com/elentok/gx/ui/explorer"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) cmdColorizeDiffs(seq int, filePath, unstagedRaw, stagedRaw string, sideBySide bool, renderWidth int) tea.Cmd {
	worktreeRoot := m.worktreeRoot
	contextLines := m.currentDiffContextLines()
	return func() tea.Msg {
		unstagedColor, _ := git.ColorizeDiff(worktreeRoot, filePath, unstagedRaw, false, sideBySide, renderWidth, contextLines)
		stagedColor, _ := git.ColorizeDiff(worktreeRoot, filePath, stagedRaw, true, sideBySide, renderWidth, contextLines)
		return diffColorizeMsg{seq: seq, filePath: filePath, unstagedRaw: unstagedRaw, unstagedColor: unstagedColor, stagedRaw: stagedRaw, stagedColor: stagedColor}
	}
}

func (m *Model) cmdColorizeUntracked(seq int, filePath, rawDiff string, sideBySide bool, renderWidth int) tea.Cmd {
	worktreeRoot := m.worktreeRoot
	contextLines := m.currentDiffContextLines()
	return func() tea.Msg {
		color, _ := git.ColorizeUntrackedDiff(worktreeRoot, filePath, rawDiff, sideBySide, renderWidth, contextLines)
		return diffColorizeMsg{seq: seq, filePath: filePath, unstagedRaw: rawDiff, unstagedColor: color}
	}
}

func (m *Model) cmdColorizeDiffsForSelection() tea.Cmd {
	sel, ok := m.selectedExplorerDiff()
	if !ok {
		return nil
	}
	file := sel.file
	seq := m.colorizeSeq
	sideBySide := m.renderMode == renderSideBySide
	renderWidth := m.deltaRenderWidth()
	if file.Untracked {
		rawDiff := strings.Join(m.unstaged.rawLines, "\n")
		return m.cmdColorizeUntracked(seq, file.Path, rawDiff, sideBySide, renderWidth)
	}
	unstagedRaw := strings.Join(m.unstaged.rawLines, "\n")
	stagedRaw := strings.Join(m.staged.rawLines, "\n")
	return m.cmdColorizeDiffs(seq, file.Path, unstagedRaw, stagedRaw, sideBySide, renderWidth)
}

type movedTarget struct {
	fromSection diffSection
	navMode     navMode
	hunkHeader  string
	lineText    string
}

func (m *Model) applySelection() tea.Cmd {
	file, ok := m.selectedExplorerFile()
	if !ok {
		return nil
	}

	sec := m.currentSection()
	sig := movedTarget{fromSection: m.section, navMode: m.navMode}
	if file.Untracked && m.section == sectionUnstaged {
		if err := git.StageIntentPath(m.worktreeRoot, file.Path); err != nil {
			m.showGitError(err)
			return nil
		}
	}

	if m.navMode == navHunk {
		if sec.activeHunk < 0 || sec.activeHunk >= len(sec.parsed.Hunks) {
			return nil
		}
		sig.hunkHeader = sec.parsed.Hunks[sec.activeHunk].Header
		patch, err := diff.BuildHunkPatch(sec.parsed, sec.activeHunk)
		if err != nil {
			m.setStatus(err.Error())
			return nil
		}
		reverse := m.section == sectionStaged
		if err := git.ApplyPatchToIndex(m.worktreeRoot, patch, reverse, false); err != nil {
			if !isCorruptPatchErr(err) {
				m.showGitError(err)
				return nil
			}
			h := sec.parsed.Hunks[sec.activeHunk]
			if len(h.ChangedLineOffset) == 0 {
				m.showGitError(err)
				return nil
			}
			startChanged := h.ChangedLineOffset[0]
			endChanged := h.ChangedLineOffset[len(h.ChangedLineOffset)-1]
			fallbackPatch, fallbackErr := diff.BuildLineRangePatch(sec.parsed, startChanged, endChanged)
			if fallbackErr != nil {
				m.showGitError(err)
				return nil
			}
			if fallbackApplyErr := git.ApplyPatchToIndex(m.worktreeRoot, fallbackPatch, reverse, true); fallbackApplyErr != nil {
				m.showGitError(err)
				return nil
			}
		}
	} else {
		if sec.activeLine < 0 || sec.activeLine >= len(sec.parsed.Changed) {
			return nil
		}
		startLine, endLine := sec.activeLine, sec.activeLine
		if sec.visualActive {
			startLine, endLine = visualLineBounds(*sec)
		}
		sig.lineText = sec.parsed.Changed[endLine].Text

		var (
			patch string
			err   error
		)
		if sec.visualActive && endLine > startLine {
			patch, err = diff.BuildLineRangePatch(sec.parsed, startLine, endLine)
		} else {
			patch, err = diff.BuildSingleLinePatch(sec.parsed, sec.activeLine)
		}
		if err != nil {
			m.setStatus(err.Error())
			return nil
		}
		reverse := m.section == sectionStaged
		if err := git.ApplyPatchToIndex(m.worktreeRoot, patch, reverse, true); err != nil {
			m.showGitError(err)
			return nil
		}
	}

	sec.visualActive = false
	sec.visualAnchor = sec.activeLine
	m.setStatus("updated " + file.Path)
	from := m.section
	m.reload(file.Path)
	if m.shouldSwitchAfterApply(from) {
		m.focusMovedTarget(sig)
		if m.flash.active {
			return nextFlashCmd()
		}
	} else {
		m.section = from
		m.ensureActiveVisible(m.currentSection())
	}
	return nil
}

func isCorruptPatchErr(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "corrupt patch")
}

func (m *Model) shouldSwitchAfterApply(from diffSection) bool {
	var sec sectionState
	if from == sectionStaged {
		sec = m.staged
	} else {
		sec = m.unstaged
	}
	return len(sec.parsed.Hunks) == 0
}

func (m *Model) reloadDiffsForSelection() tea.Cmd {
	m.colorizeSeq++
	seq := m.colorizeSeq
	sideBySide := m.renderMode == renderSideBySide
	renderWidth := m.deltaRenderWidth()

	sel, ok := m.selectedExplorerDiff()
	if !ok {
		m.activeFilePath = ""
		m.unstaged = newSectionState()
		m.staged = newSectionState()
		m.syncDiffViewports()
		if strings.TrimSpace(m.searchQuery) != "" && (m.searchScope == searchScopeUnstaged || m.searchScope == searchScopeStaged) {
			m.recomputeSearchMatches()
		}
		return nil
	}

	file := sel.file
	if file.Path != m.activeFilePath {
		m.section = sectionUnstaged
		m.activeFilePath = file.Path
	}
	if file.Untracked {
		raw, err := git.DiffUntrackedPath(m.worktreeRoot, file.Path, false, false, 0, m.currentDiffContextLines())
		if err != nil {
			m.showGitError(err)
			raw = ""
		}
		m.unstaged = buildSectionState(raw, raw, m.unstaged, sideBySide)
		m.staged = newSectionState()
		m.section = sectionUnstaged
		m.syncDiffViewports()
		return m.cmdColorizeUntracked(seq, file.Path, raw, sideBySide, renderWidth)
	}

	unstagedRaw, err := git.DiffPath(m.worktreeRoot, file.Path, false, m.currentDiffContextLines())
	if err != nil {
		m.showGitError(err)
		unstagedRaw = ""
	}
	stagedRaw, err := git.DiffPath(m.worktreeRoot, file.Path, true, m.currentDiffContextLines())
	if err != nil {
		m.showGitError(err)
		stagedRaw = ""
	}

	m.unstaged = buildSectionState(unstagedRaw, unstagedRaw, m.unstaged, sideBySide)
	m.staged = buildSectionState(stagedRaw, stagedRaw, m.staged, sideBySide)
	m.pickAvailableSection()
	m.syncDiffViewports()
	if strings.TrimSpace(m.searchQuery) != "" && (m.searchScope == searchScopeUnstaged || m.searchScope == searchScopeStaged) {
		m.recomputeSearchMatches()
	}
	return m.cmdColorizeDiffs(seq, file.Path, unstagedRaw, stagedRaw, sideBySide, renderWidth)
}

func (m *Model) enterDiffFromStatus(resetSection bool) tea.Cmd {
	if _, ok := m.selectedExplorerFile(); !ok {
		return nil
	}
	m.diffReloadSeq++
	cmd := m.reloadDiffsForSelection()
	m.focus = focusDiff
	if resetSection {
		m.section = sectionUnstaged
	}
	m.pickAvailableSection()
	m.syncDiffViewports()
	m.ensureActiveVisible(m.currentSection())
	return cmd
}

func (m *Model) openDiscardDiffConfirm() {
	if m.section != sectionUnstaged {
		return
	}
	file, ok := m.selectedExplorerFile()
	if !ok {
		return
	}
	sec := m.currentSection()

	var (
		title       string
		lines       []string
		patch       string
		unidiffZero bool
		err         error
	)

	if m.navMode == navHunk {
		if sec.activeHunk < 0 || sec.activeHunk >= len(sec.parsed.Hunks) {
			return
		}
		patch, err = diff.BuildHunkPatch(sec.parsed, sec.activeHunk)
		title = "Discard selected hunk?"
		lines = []string{"This will discard the selected hunk from your working tree."}
	} else {
		if sec.activeLine < 0 || sec.activeLine >= len(sec.parsed.Changed) {
			return
		}
		startLine, endLine := sec.activeLine, sec.activeLine
		if sec.visualActive {
			startLine, endLine = visualLineBounds(*sec)
		}
		if sec.visualActive && endLine > startLine {
			patch, err = diff.BuildLineRangePatch(sec.parsed, startLine, endLine)
			title = "Discard selected lines?"
			lines = []string{"This will discard the selected lines from your working tree."}
		} else {
			patch, err = diff.BuildSingleLinePatch(sec.parsed, sec.activeLine)
			title = "Discard selected line?"
			lines = []string{"This will discard the selected line from your working tree."}
		}
		unidiffZero = true
	}

	if err != nil {
		m.setStatus(err.Error())
		return
	}

	m.openConfirm(title, lines, confirmDiscardUnstaged, "", "")
	m.confirmPaths = []string{file.Path}
	m.confirmPatch = patch
	m.confirmPatchUnidiffZero = unidiffZero
}

func buildSectionState(raw, color string, prev sectionState, sideBySide bool) sectionState {
	data := explorer.BuildSectionData(raw, color, toExplorerSectionData(prev), sideBySide)
	state := fromExplorerSectionData(data, prev.viewport)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		state.viewport.SetContent("")
		state.viewport.SetYOffset(0)
		return state
	}
	prevOffset := state.viewport.YOffset()
	state.viewport.SetContentLines(state.viewLines)
	state.viewport.SetYOffset(prevOffset)
	return state
}

func (m *Model) focusMovedTarget(sig movedTarget) {
	if sig.fromSection == sectionUnstaged {
		m.section = sectionStaged
	} else {
		m.section = sectionUnstaged
	}
	var sec *sectionState
	if m.section == sectionStaged {
		sec = &m.staged
	} else {
		sec = &m.unstaged
	}

	if sig.navMode == navHunk {
		for i := range sec.parsed.Hunks {
			if sec.parsed.Hunks[i].Header == sig.hunkHeader {
				sec.activeHunk = i
				m.ensureActiveVisible(sec)
				m.flash = flashState{active: true, section: m.section, navMode: navHunk, hunk: i, frames: 4}
				return
			}
		}
		if len(sec.parsed.Hunks) > 0 {
			sec.activeHunk = 0
			m.ensureActiveVisible(sec)
			m.flash = flashState{active: true, section: m.section, navMode: navHunk, hunk: 0, frames: 4}
		}
		return
	}

	for i := range sec.parsed.Changed {
		if sec.parsed.Changed[i].Text == sig.lineText {
			sec.activeLine = i
			m.ensureActiveVisible(sec)
			m.flash = flashState{active: true, section: m.section, navMode: navLine, line: i, frames: 4}
			return
		}
	}
	if len(sec.parsed.Changed) > 0 {
		sec.activeLine = 0
		m.ensureActiveVisible(sec)
		m.flash = flashState{active: true, section: m.section, navMode: navLine, line: 0, frames: 4}
	}
}

func isDeltaSectionDivider(plain string) bool {
	return explorer.IsDeltaSectionDivider(plain)
}

func splitLines(s string) []string {
	return explorer.SplitLines(s)
}
