package status

import (
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/diff"
	"github.com/elentok/gx/ui/explorer"

	tea "charm.land/bubbletea/v2"
)

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

func (m *Model) reloadDiffsForSelection() {
	sel, ok := m.selectedExplorerDiff()
	if !ok {
		m.activeFilePath = ""
		m.unstaged = newSectionState()
		m.staged = newSectionState()
		m.syncDiffViewports()
		if strings.TrimSpace(m.searchQuery) != "" && (m.searchScope == searchScopeUnstaged || m.searchScope == searchScopeStaged) {
			m.recomputeSearchMatches()
		}
		return
	}

	file := sel.file
	if file.Path != m.activeFilePath {
		m.section = sectionUnstaged
		m.activeFilePath = file.Path
	}
	if file.Untracked {
		renderWidth := m.deltaRenderWidth()
		raw, err := git.DiffUntrackedPath(m.worktreeRoot, file.Path, false, false, 0, m.currentDiffContextLines())
		if err != nil {
			m.showGitError(err)
			raw = ""
		}
		col, err := git.DiffUntrackedPath(m.worktreeRoot, file.Path, true, m.renderMode == renderSideBySide, renderWidth, m.currentDiffContextLines())
		if err != nil {
			col = raw
		}
		m.unstaged = buildSectionState(raw, col, m.unstaged, m.renderMode == renderSideBySide)
		m.staged = newSectionState()
		m.section = sectionUnstaged
		m.syncDiffViewports()
		return
	}

	unstagedRaw, err := git.DiffPath(m.worktreeRoot, file.Path, false, m.currentDiffContextLines())
	if err != nil {
		m.showGitError(err)
		unstagedRaw = ""
	}
	renderWidth := m.deltaRenderWidth()
	unstagedColor, err := git.DiffPathWithDelta(m.worktreeRoot, file.Path, false, m.renderMode == renderSideBySide, renderWidth, m.currentDiffContextLines())
	if err != nil {
		unstagedColor = unstagedRaw
	}

	stagedRaw, err := git.DiffPath(m.worktreeRoot, file.Path, true, m.currentDiffContextLines())
	if err != nil {
		m.showGitError(err)
		stagedRaw = ""
	}
	stagedColor, err := git.DiffPathWithDelta(m.worktreeRoot, file.Path, true, m.renderMode == renderSideBySide, renderWidth, m.currentDiffContextLines())
	if err != nil {
		stagedColor = stagedRaw
	}

	m.unstaged = buildSectionState(unstagedRaw, unstagedColor, m.unstaged, m.renderMode == renderSideBySide)
	m.staged = buildSectionState(stagedRaw, stagedColor, m.staged, m.renderMode == renderSideBySide)
	m.pickAvailableSection()
	m.syncDiffViewports()
	if strings.TrimSpace(m.searchQuery) != "" && (m.searchScope == searchScopeUnstaged || m.searchScope == searchScopeStaged) {
		m.recomputeSearchMatches()
	}
}

func (m *Model) enterDiffFromStatus(resetSection bool) {
	if _, ok := m.selectedExplorerFile(); !ok {
		return
	}
	m.diffReloadSeq++
	m.reloadDiffsForSelection()
	m.focus = focusDiff
	if resetSection {
		m.section = sectionUnstaged
	}
	m.pickAvailableSection()
	m.syncDiffViewports()
	m.ensureActiveVisible(m.currentSection())
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
	state := sectionState{activeHunk: prev.activeHunk, activeLine: prev.activeLine, visualActive: prev.visualActive, visualAnchor: prev.visualAnchor, viewport: prev.viewport}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		state.activeHunk = -1
		state.activeLine = -1
		state.baseLines = nil
		state.baseLineKinds = nil
		state.baseDisplayToRaw = nil
		state.viewLines = nil
		state.viewLineKinds = nil
		state.displayToRaw = nil
		state.rawToDisplay = nil
		state.hunkDisplayRange = nil
		state.changedDisplay = nil
		state.visualActive = false
		state.visualAnchor = -1
		state.viewport.SetContent("")
		state.viewport.SetYOffset(0)
		return state
	}

	state.parsed = diff.ParseUnifiedDiff(raw)
	state.rawLines = append([]string{}, state.parsed.Lines...)
	if sideBySide {
		initSideBySideSectionState(&state, color)
		return state
	}

	colorLines := explorer.SplitLines(color)
	if len(colorLines) == 0 {
		colorLines = append([]string{}, state.rawLines...)
	} else if len(colorLines) < len(state.rawLines) {
		colorLines = append(colorLines, state.rawLines[len(colorLines):]...)
	} else if len(colorLines) > len(state.rawLines) {
		colorLines = colorLines[:len(state.rawLines)]
	}
	state.baseLines, state.baseLineKinds, state.baseDisplayToRaw = diff.BuildDisplayBaseLines(state.parsed, colorLines)
	state.viewLines = append([]string{}, state.baseLines...)
	state.viewLineKinds = append([]diffDisplayRowKind{}, state.baseLineKinds...)
	state.displayToRaw = append([]int{}, state.baseDisplayToRaw...)
	state.rawToDisplay = diff.BuildRawToDisplayMap(state.parsed, state.displayToRaw)
	state.hunkDisplayRange = nil
	state.changedDisplay = nil
	prevOffset := state.viewport.YOffset()
	state.viewport.SetContentLines(state.viewLines)
	state.viewport.SetYOffset(prevOffset)

	if len(state.parsed.Hunks) == 0 {
		state.activeHunk = -1
	} else {
		if state.activeHunk < 0 {
			state.activeHunk = 0
		}
		if state.activeHunk >= len(state.parsed.Hunks) {
			state.activeHunk = len(state.parsed.Hunks) - 1
		}
	}

	if len(state.parsed.Changed) == 0 {
		state.activeLine = -1
		state.visualActive = false
		state.visualAnchor = -1
	} else {
		if state.activeLine < 0 {
			state.activeLine = 0
		}
		if state.activeLine >= len(state.parsed.Changed) {
			state.activeLine = len(state.parsed.Changed) - 1
		}
		if state.visualAnchor < 0 || state.visualAnchor >= len(state.parsed.Changed) {
			state.visualAnchor = state.activeLine
		}
	}

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

func initSideBySideSectionState(state *sectionState, color string) {
	state.viewLines = explorer.SplitLines(color)
	if len(state.viewLines) == 0 {
		state.viewLines = append([]string{}, state.rawLines...)
	}
	state.baseLines = append([]string{}, state.viewLines...)
	state.baseLineKinds = make([]diffDisplayRowKind, len(state.baseLines))
	state.baseDisplayToRaw = make([]int, len(state.baseLines))
	for i := range state.baseDisplayToRaw {
		state.baseDisplayToRaw[i] = -1
	}
	state.viewLineKinds = append([]diffDisplayRowKind{}, state.baseLineKinds...)
	state.displayToRaw = append([]int{}, state.baseDisplayToRaw...)
	state.changedDisplay = make([]int, len(state.parsed.Changed))
	for i := range state.changedDisplay {
		state.changedDisplay[i] = -1
	}
	mapping := explorer.BuildSideBySideMapping(state.parsed, state.viewLines)
	state.displayToRaw = mapping.DisplayToRaw
	state.rawToDisplay = mapping.RawToDisplay
	state.changedDisplay = mapping.ChangedDisplay
	state.hunkDisplayRange = mapping.HunkDisplayRange
	prevOffset := state.viewport.YOffset()
	state.viewport.SetContentLines(state.viewLines)
	state.viewport.SetYOffset(prevOffset)

	if len(state.parsed.Hunks) == 0 {
		state.activeHunk = -1
	} else {
		if state.activeHunk < 0 {
			state.activeHunk = 0
		}
		if state.activeHunk >= len(state.parsed.Hunks) {
			state.activeHunk = len(state.parsed.Hunks) - 1
		}
	}

	if len(state.parsed.Changed) == 0 {
		state.activeLine = -1
		state.visualActive = false
		state.visualAnchor = -1
	} else {
		if state.activeLine < 0 {
			state.activeLine = 0
		}
		if state.activeLine >= len(state.parsed.Changed) {
			state.activeLine = len(state.parsed.Changed) - 1
		}
		if state.visualAnchor < 0 || state.visualAnchor >= len(state.parsed.Changed) {
			state.visualAnchor = state.activeLine
		}
	}
}

func isDeltaSectionDivider(plain string) bool {
	return explorer.IsDeltaSectionDivider(plain)
}

func splitLines(s string) []string {
	return explorer.SplitLines(s)
}
