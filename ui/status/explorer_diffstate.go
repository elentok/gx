package status

import (
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui/diff/diffcore"
	"github.com/elentok/gx/ui/explorer"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) colorizeDiffsSync(filePath, unstagedRaw, stagedRaw string, sideBySide bool, renderWidth int) (unstagedColor, stagedColor string) {
	contextLines := m.currentDiffContextLines()
	unstagedColor, _ = git.ColorizeDiff(m.worktreeRoot, filePath, unstagedRaw, false, sideBySide, renderWidth, contextLines)
	stagedColor, _ = git.ColorizeDiff(m.worktreeRoot, filePath, stagedRaw, true, sideBySide, renderWidth, contextLines)
	return unstagedColor, stagedColor
}

func (m *Model) colorizeUntrackedSync(filePath, rawDiff string, sideBySide bool, renderWidth int) string {
	contextLines := m.currentDiffContextLines()
	color, _ := git.ColorizeUntrackedDiff(m.worktreeRoot, filePath, rawDiff, sideBySide, renderWidth, contextLines)
	return color
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
		if sec.data.ActiveHunk < 0 || sec.data.ActiveHunk >= len(sec.data.Parsed.Hunks) {
			return nil
		}
		sig.hunkHeader = sec.data.Parsed.Hunks[sec.data.ActiveHunk].Header
		patch, err := diffcore.BuildHunkPatch(sec.data.Parsed, sec.data.ActiveHunk)
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
			h := sec.data.Parsed.Hunks[sec.data.ActiveHunk]
			if len(h.ChangedLineOffset) == 0 {
				m.showGitError(err)
				return nil
			}
			startChanged := h.ChangedLineOffset[0]
			endChanged := h.ChangedLineOffset[len(h.ChangedLineOffset)-1]
			fallbackPatch, fallbackErr := diffcore.BuildLineRangePatch(sec.data.Parsed, startChanged, endChanged)
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
		if sec.data.ActiveLine < 0 || sec.data.ActiveLine >= len(sec.data.Parsed.Changed) {
			return nil
		}
		startLine, endLine := sec.data.ActiveLine, sec.data.ActiveLine
		if sec.data.VisualActive {
			startLine, endLine = visualLineBounds(*sec)
		}
		sig.lineText = sec.data.Parsed.Changed[endLine].Text

		var (
			patch string
			err   error
		)
		if sec.data.VisualActive && endLine > startLine {
			patch, err = diffcore.BuildLineRangePatch(sec.data.Parsed, startLine, endLine)
		} else {
			patch, err = diffcore.BuildSingleLinePatch(sec.data.Parsed, sec.data.ActiveLine)
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

	sec.data.VisualActive = false
	sec.data.VisualAnchor = sec.data.ActiveLine
	m.setStatus("updated " + file.Path)
	from := m.section
	reloadCmd := m.reload(file.Path)
	if m.shouldSwitchAfterApply(from) {
		m.focusMovedTarget(sig)
		if m.flash.active {
			return tea.Batch(reloadCmd, nextFlashCmd())
		}
	} else {
		m.section = from
		m.ensureActiveVisible(m.currentSection())
	}
	return reloadCmd
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
	return len(sec.data.Parsed.Hunks) == 0
}

func (m *Model) reloadDiffsForSelection() tea.Cmd {
	m.syncDiffModels()
	sideBySide := m.renderMode == renderSideBySide
	renderWidth := m.deltaRenderWidth()

	sel, ok := m.selectedExplorerDiff()
	if !ok {
		m.activeFilePath = ""
		m.unstaged = newSectionState()
		m.staged = newSectionState()
		m.syncDiffModels()
		m.syncDiffViewports()
		if m.currentDiffSearch().HasQuery() && (m.currentSearchScope() == searchScopeUnstaged || m.currentSearchScope() == searchScopeStaged) {
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
		color := m.colorizeUntrackedSync(file.Path, raw, sideBySide, renderWidth)
		if color == "" {
			color = raw
		}
		m.unstagedDiffModel.BuildFromRaw(raw, color, sideBySide)
		m.stagedDiffModel.BuildFromRaw("", "", sideBySide)
		m.syncSectionsFromDiffModels()
		m.unstaged.colorized = true
		m.staged = newSectionState()
		m.stagedDiffModel.SetData(m.staged.data)
		m.section = sectionUnstaged
		m.syncDiffViewports()
		return nil
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

	unstagedColor, stagedColor := m.colorizeDiffsSync(file.Path, unstagedRaw, stagedRaw, sideBySide, renderWidth)
	if unstagedColor == "" {
		unstagedColor = unstagedRaw
	}
	if stagedColor == "" {
		stagedColor = stagedRaw
	}
	m.unstagedDiffModel.BuildFromRaw(unstagedRaw, unstagedColor, sideBySide)
	m.stagedDiffModel.BuildFromRaw(stagedRaw, stagedColor, sideBySide)
	m.syncSectionsFromDiffModels()
	m.unstaged.colorized = true
	m.staged.colorized = true
	m.pickAvailableSection()
	m.syncDiffViewports()
	if m.currentDiffSearch().HasQuery() && (m.currentSearchScope() == searchScopeUnstaged || m.currentSearchScope() == searchScopeStaged) {
		m.recomputeSearchMatches()
	}
	return nil
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
		if sec.data.ActiveHunk < 0 || sec.data.ActiveHunk >= len(sec.data.Parsed.Hunks) {
			return
		}
		patch, err = diffcore.BuildHunkPatch(sec.data.Parsed, sec.data.ActiveHunk)
		title = "Discard selected hunk?"
		lines = []string{"This will discard the selected hunk from your working tree."}
	} else {
		if sec.data.ActiveLine < 0 || sec.data.ActiveLine >= len(sec.data.Parsed.Changed) {
			return
		}
		startLine, endLine := sec.data.ActiveLine, sec.data.ActiveLine
		if sec.data.VisualActive {
			startLine, endLine = visualLineBounds(*sec)
		}
		if sec.data.VisualActive && endLine > startLine {
			patch, err = diffcore.BuildLineRangePatch(sec.data.Parsed, startLine, endLine)
			title = "Discard selected lines?"
			lines = []string{"This will discard the selected lines from your working tree."}
		} else {
			patch, err = diffcore.BuildSingleLinePatch(sec.data.Parsed, sec.data.ActiveLine)
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
		for i := range sec.data.Parsed.Hunks {
			if sec.data.Parsed.Hunks[i].Header == sig.hunkHeader {
				sec.data.ActiveHunk = i
				m.ensureActiveVisible(sec)
				m.flash = flashState{active: true, section: m.section, navMode: navHunk, hunk: i, frames: 4}
				return
			}
		}
		if len(sec.data.Parsed.Hunks) > 0 {
			sec.data.ActiveHunk = 0
			m.ensureActiveVisible(sec)
			m.flash = flashState{active: true, section: m.section, navMode: navHunk, hunk: 0, frames: 4}
		}
		return
	}

	for i := range sec.data.Parsed.Changed {
		if sec.data.Parsed.Changed[i].Text == sig.lineText {
			sec.data.ActiveLine = i
			m.ensureActiveVisible(sec)
			m.flash = flashState{active: true, section: m.section, navMode: navLine, line: i, frames: 4}
			return
		}
	}
	if len(sec.data.Parsed.Changed) > 0 {
		sec.data.ActiveLine = 0
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
